package bench

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// The four answers the green half gives for one changed test. Only a witness is evidence of a defect: it is
// red on HEAD's code and green with the candidate fix.
const (
	FalsifyWitness     = "witness"
	FalsifyWrongReason = "wrong_reason"
	FalsifyPinsNothing = "pins_nothing"
	FalsifyInverted    = "inverted"
)

// falsifyGitlinkMode is the mode git gives a submodule entry: a path whose content is another repository's
// commit, which no patch can carry and no restore can put back.
const falsifyGitlinkMode = "160000"

// falsifyBeforeRestore observes the saved patch right before the restore starts. The patch is the only copy of
// the candidate fix from that moment on, so a test drives this seam to prove the save finished first.
var falsifyBeforeRestore func(patch string)

// falsifyNow names the saved patch. A test pins it to land two saves on the same name.
var falsifyNow = time.Now

// FalsifyVerdict is one changed test's answer: whether it fails on HEAD's code, and whether the candidate fix
// makes it pass.
type FalsifyVerdict struct {
	Test    string `json:"test"`
	Verdict string `json:"verdict"`
}

// FalsifyReport is what the falsify check found: the verdict per changed test, the patch the candidate fix was
// saved to, and the source files the command restored to HEAD.
type FalsifyReport struct {
	Suite       string           `json:"suite"`
	TestFiles   []string         `json:"test_files"`
	SourceFiles []string         `json:"source_files"`
	Verdicts    []FalsifyVerdict `json:"verdicts"`
	Uncollected []Uncollected    `json:"uncollected,omitempty"`
	Patch       string           `json:"patch,omitempty"`
	Restored    []string         `json:"restored,omitempty"`
	Left        []string         `json:"left,omitempty"`
	Notes       []string         `json:"notes,omitempty"`
}

// Falsify judges the green half of a witness. RedCheck runs the changed tests on HEAD's code; Falsify also
// runs them on the working tree, which carries the agent's source change as the candidate fix, and answers
// per test: witness (red at HEAD, green with the fix), wrong_reason (red both), pins_nothing (green at HEAD)
// or inverted (green at HEAD, red with the fix). Before restoring anything it saves the source diff, including
// new untracked source files, under <git-dir>/tsp/falsify/, then restores exactly those source files to HEAD
// and proves the restore. Test files are never touched. A save that fails restores nothing: the working tree
// is the only copy of the fix until the patch holds it.
func Falsify(dir, suite string, timeout time.Duration) (FalsifyReport, error) {
	var rep FalsifyReport
	root, err := falsifyRoot(dir)
	if err != nil {
		return rep, err
	}
	change, err := falsifyRead(root)
	if err != nil {
		return rep, err
	}
	rep.TestFiles, rep.SourceFiles = change.testFiles, change.sourceFiles
	if len(change.sourceFiles) == 0 {
		return rep, errors.New("no source change against HEAD: write the candidate fix in the source first")
	}
	// Fail closed on the changes neither the patch nor the restore can hold. A staged version that differs from
	// the worktree has no copy in the patch, which is the worktree alone, and a submodule is not a file the
	// restore can put back at HEAD. The fix is the only copy, so nothing is saved or restored either way.
	if len(change.stagedConflicts) > 0 {
		return rep, fmt.Errorf("the index holds a different version of %s than the worktree: stage or unstage consistently, then rerun; nothing was saved or restored",
			strings.Join(change.stagedConflicts, ", "))
	}
	if links := change.gitlinks(); len(links) > 0 {
		return rep, fmt.Errorf("submodule changes are not supported: %s; commit or drop the submodule change, then rerun; nothing was saved or restored",
			strings.Join(links, ", "))
	}
	patch, err := falsifyPatch(root, change)
	if err != nil {
		rep.Left = falsifyStillChanged(root, change.sourceFiles)
		return rep, fmt.Errorf("read the candidate fix: %v; nothing was restored, the source still differs from HEAD", err)
	}
	path, err := falsifySavePatch(root, patch)
	if err != nil {
		rep.Left = falsifyStillChanged(root, change.sourceFiles)
		return rep, fmt.Errorf("save the candidate fix to %s: %v; nothing was restored, %d source file(s) still differ from HEAD: %s",
			path, err, len(change.sourceFiles), strings.Join(change.sourceFiles, ", "))
	}
	// Only a save that landed names a patch: the report is what the caller prints as saved, and a path that
	// holds nothing would read as a copy of a fix that only the working tree still has.
	rep.Patch = path
	var verdicts []FalsifyVerdict
	var notes []string
	var judgeErr error
	if len(change.testFiles) > 0 {
		red, before, _, _, redErr := redCheckDetail(root, suite, "HEAD", change.testFiles, timeout)
		rep.Suite, rep.Uncollected = red.Suite, red.Uncollected
		if redErr != nil {
			judgeErr = redErr
		} else {
			verdicts, notes, judgeErr = falsifyVerdicts(root, change.testFiles, red, before, timeout)
		}
	}
	rep.Notes, rep.Verdicts = notes, verdicts
	restored, left, err := falsifyRestoreAndProve(root, change, path)
	rep.Restored, rep.Left = restored, left
	if err != nil {
		return rep, err
	}
	if judgeErr != nil {
		return rep, judgeErr
	}
	if len(change.testFiles) == 0 {
		return rep, nil
	}
	if len(verdicts) == 0 {
		return rep, fmt.Errorf("no changed test could be judged with the candidate fix; saved candidate fix: %s", rep.Patch)
	}
	return rep, nil
}

// falsifyRestoreAndProve restores the saved source files and proves it against two whole readings of the
// working-tree status: only the paths the command saved may change between them, so a restore that rewrote any
// other file cannot pass as proven. A tree it cannot read back is never reported as restored.
func falsifyRestoreAndProve(root string, change falsifyChange, patch string) (restored, left []string, err error) {
	statusBefore, err := falsifyStatus(root)
	if err != nil {
		return nil, falsifyStillChanged(root, change.sourceFiles),
			fmt.Errorf("read the working tree before the restore: %v; saved candidate fix: %s", err, patch)
	}
	if falsifyBeforeRestore != nil {
		falsifyBeforeRestore(patch)
	}
	restored, restoreErr := falsifyRestore(root, change)
	statusAfter, statusErr := falsifyStatus(root)
	if statusErr != nil {
		return restored, falsifyStillChanged(root, change.sourceFiles),
			fmt.Errorf("prove the restore with git status: %v; saved candidate fix: %s; restore error: %v", statusErr, patch, restoreErr)
	}
	left = falsifySourceLeft(statusAfter, change.sourceFiles)
	if restoreErr != nil {
		return restored, left, fmt.Errorf("restore the source: %v; saved candidate fix: %s; still changed: %s",
			restoreErr, patch, strings.Join(left, ", "))
	}
	if outside := falsifyOutside(statusBefore, statusAfter, setOf(change.sourceFiles)); len(outside) > 0 {
		return restored, left, fmt.Errorf("the restore changed files outside the saved source set: %s; saved candidate fix: %s; restored: %s",
			strings.Join(outside, ", "), patch, strings.Join(restored, ", "))
	}
	if len(left) > 0 {
		return restored, left, fmt.Errorf("the working tree still differs from HEAD in %s; saved candidate fix: %s",
			strings.Join(left, ", "), patch)
	}
	return restored, nil, nil
}

func falsifyRoot(dir string) (string, error) {
	out, err := gitFalsify(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("not inside a git repository: %s", strings.TrimSpace(out))
	}
	return strings.TrimSpace(out), nil
}

// falsifyChange is the working-tree change against HEAD, split by what the command does with it and by what it
// cannot handle at all.
type falsifyChange struct {
	testFiles   []string
	sourceFiles []string
	untracked   map[string]bool
	headModes   map[string]string
	indexModes  map[string]string
	// stagedConflicts are the source files whose index entry differs from both HEAD and the worktree.
	stagedConflicts []string
}

// gitlinks names the changed source paths HEAD or the index record as a submodule.
func (c falsifyChange) gitlinks() []string {
	var links []string
	for _, rel := range c.sourceFiles {
		if c.headModes[rel] == falsifyGitlinkMode || c.indexModes[rel] == falsifyGitlinkMode {
			links = append(links, rel)
		}
	}
	sort.Strings(links)
	return links
}

// falsifyRead splits the working-tree change against HEAD into test files and source files. A deleted file is
// still part of the change, and an untracked file is the change too when git does not ignore it. The modes and
// the index's own change are what tell a submodule and a staged-and-unstaged source file from a plain edit.
func falsifyRead(root string) (falsifyChange, error) {
	var change falsifyChange
	trackedOut, err := gitFalsify(root, "diff", "--name-only", "--no-renames", "-z", "HEAD")
	if err != nil {
		return change, fmt.Errorf("read the working-tree change: %v: %s", err, strings.TrimSpace(trackedOut))
	}
	untrackedOut, err := gitFalsify(root, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return change, fmt.Errorf("list the untracked files: %v: %s", err, strings.TrimSpace(untrackedOut))
	}
	stagedOut, err := gitFalsify(root, "diff", "--cached", "--name-only", "--no-renames", "-z", "HEAD")
	if err != nil {
		return change, fmt.Errorf("read the index's change: %v: %s", err, strings.TrimSpace(stagedOut))
	}
	unstagedOut, err := gitFalsify(root, "diff", "--name-only", "--no-renames", "-z")
	if err != nil {
		return change, fmt.Errorf("read the worktree's change: %v: %s", err, strings.TrimSpace(unstagedOut))
	}
	headOut, err := gitFalsify(root, "ls-tree", "-r", "-z", "HEAD")
	if err != nil {
		return change, fmt.Errorf("read HEAD's files: %v: %s", err, strings.TrimSpace(headOut))
	}
	indexOut, err := gitFalsify(root, "ls-files", "-s", "-z")
	if err != nil {
		return change, fmt.Errorf("read the index's files: %v: %s", err, strings.TrimSpace(indexOut))
	}
	change.headModes, change.indexModes = gitModes(headOut), gitModes(indexOut)
	change.untracked = setOf(splitNul(untrackedOut))
	staged, unstaged := setOf(splitNul(stagedOut)), setOf(splitNul(unstagedOut))
	seen := map[string]bool{}
	for _, p := range append(splitNul(trackedOut), splitNul(untrackedOut)...) {
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		if isTestFile(p) {
			change.testFiles = append(change.testFiles, p)
		} else {
			change.sourceFiles = append(change.sourceFiles, p)
		}
	}
	sort.Strings(change.testFiles)
	sort.Strings(change.sourceFiles)
	// A source file with both a staged and an unstaged edit has three versions: HEAD's, the index's and the
	// worktree's. The patch holds the worktree one and the restore resets the index, so the index's version
	// would be left nowhere.
	for _, rel := range change.sourceFiles {
		if staged[rel] && unstaged[rel] {
			change.stagedConflicts = append(change.stagedConflicts, rel)
		}
	}
	sort.Strings(change.stagedConflicts)
	return change, nil
}

// gitModes reads the mode of every path from the entries ls-tree and ls-files print, which is how a submodule
// entry is told from a file. A path both commands hold appears in both maps.
func gitModes(out string) map[string]string {
	entries := map[string]string{}
	for _, entry := range splitNul(out) {
		mode, rest, ok := strings.Cut(entry, " ")
		if !ok {
			continue
		}
		if _, path, ok := strings.Cut(rest, "\t"); ok {
			entries[path] = mode
		}
	}
	return entries
}

// falsifyPatch is the candidate fix as one binary-safe patch: `git diff HEAD` for everything git tracks,
// including a staged new file, plus a no-index diff for each new source file git has never seen.
func falsifyPatch(root string, change falsifyChange) ([]byte, error) {
	var tracked, fresh []string
	for _, rel := range change.sourceFiles {
		if change.untracked[rel] {
			fresh = append(fresh, rel)
		} else {
			tracked = append(tracked, rel)
		}
	}
	var patch bytes.Buffer
	if len(tracked) > 0 {
		out, err := gitFalsifyBytes(root, append([]string{"diff", "--binary", "--no-renames", "HEAD", "--"}, tracked...)...)
		if err != nil {
			return nil, err
		}
		patch.Write(out)
	}
	for _, rel := range fresh {
		out, err := gitFalsifyBytes(root, "diff", "--no-index", "--binary", "--", "/dev/null", rel)
		if err != nil {
			return nil, err
		}
		patch.Write(out)
	}
	if patch.Len() == 0 {
		return nil, errors.New("the source change produced an empty patch")
	}
	return patch.Bytes(), nil
}

// falsifySavePatch writes the patch under <git-dir>/tsp/falsify and makes it durable before the caller restores
// anything: the bytes go to a temp file beside the final name, that file is synced, renamed into place, and the
// directory is synced. The final name is created exclusively, so two runs that start in the same nanosecond
// cannot overwrite one another; a name another run owns gains a suffix instead.
func falsifySavePatch(root string, patch []byte) (path string, err error) {
	dir, err := falsifyPatchDir(root)
	if err != nil {
		return "", err
	}
	base := falsifyNow().UTC().Format("20060102T150405.000000000Z")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return filepath.Join(dir, base+".patch"), err
	}
	tmp, err := os.CreateTemp(dir, base+".*.tmp")
	if err != nil {
		return filepath.Join(dir, base+".patch"), err
	}
	tmpName := tmp.Name()
	// A failed save leaves no half-written file behind to be mistaken for the fix.
	defer func() {
		if err == nil {
			return
		}
		for _, stale := range []string{tmpName, path} {
			discard(stale)
		}
	}()
	if _, err = tmp.Write(patch); err != nil {
		tmp.Close()
		return filepath.Join(dir, base+".patch"), err
	}
	if err = tmp.Sync(); err != nil {
		tmp.Close()
		return filepath.Join(dir, base+".patch"), err
	}
	if err = tmp.Close(); err != nil {
		return filepath.Join(dir, base+".patch"), err
	}
	if path, err = falsifyReservePatch(dir, base); err != nil {
		return path, err
	}
	if err = os.Rename(tmpName, path); err != nil {
		return path, err
	}
	if err = syncDir(dir); err != nil {
		return path, err
	}
	return path, nil
}

// falsifyPatchDir is <git-dir>/tsp/falsify. The git dir is resolved by git, so a linked worktree saves into its
// own git dir instead of a .git beside the sources.
func falsifyPatchDir(root string) (string, error) {
	out, err := gitFalsify(root, "rev-parse", "--git-dir")
	if err != nil {
		return "", fmt.Errorf("locate the git dir: %v: %s", err, strings.TrimSpace(out))
	}
	gitDir := strings.TrimSpace(out)
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(root, gitDir)
	}
	return filepath.Join(gitDir, "tsp", "falsify"), nil
}

// falsifyReservePatch creates the patch's final name exclusively: EEXIST means another run already owns that
// name, so it takes the next suffix instead of overwriting the fix that run saved.
func falsifyReservePatch(dir, base string) (string, error) {
	for i := 0; i < 1000; i++ {
		name := base + ".patch"
		if i > 0 {
			name = fmt.Sprintf("%s-%d.patch", base, i)
		}
		path := filepath.Join(dir, name)
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			return path, f.Close()
		}
		if !errors.Is(err, os.ErrExist) {
			return path, err
		}
	}
	return filepath.Join(dir, base+".patch"), errors.New("no free patch name: 1000 runs share one timestamp")
}

// syncDir syncs a directory so a rename into it survives a crash.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	if err := d.Sync(); err != nil {
		d.Close()
		return err
	}
	return d.Close()
}

// discard removes a file a failed save wrote. It reports nothing: the save is already failing, and a stale
// file under the git dir is the smaller problem.
func discard(path string) {
	if path != "" {
		_ = os.Remove(path)
	}
}

// falsifyVerdicts runs the same changed tests on the working tree, which carries the candidate fix, and
// compares each outcome with the one HEAD's code gave.
func falsifyVerdicts(root string, testFiles []string, red RedCheckReport, before map[string]bool, timeout time.Duration) ([]FalsifyVerdict, []string, error) {
	green, _, out, err := perTest(root, red.Suite, timeout, testFiles...)
	if err != nil {
		return nil, nil, fmt.Errorf("run the changed tests with the candidate fix: %v", err)
	}
	if _, whole := green["suite"]; len(green) == 0 || (len(green) == 1 && whole) {
		return nil, nil, fmt.Errorf("the changed tests could not be judged with the candidate fix: %s", tail(out, 600))
	}
	redAtHead := map[string]bool{}
	names := map[string]bool{}
	for _, name := range red.Red {
		names[name] = true
		redAtHead[name] = true
	}
	for _, name := range red.PinsNothing {
		names[name] = true
	}
	// A package HEAD cannot build reports no outcome per test, so redcheck calls it a witness before it names
	// any test. Falsify names the tests: those the fix's run reports, minus the ones that already ran at HEAD.
	for _, pkg := range red.NewCode {
		for name := range green {
			if _, existed := before[name]; !existed && packageTestCovers(pkg, name) {
				names[name] = true
				redAtHead[name] = true
			}
		}
	}
	var verdicts []FalsifyVerdict
	var missing []string
	for name := range names {
		passed, ran := green[name]
		if !ran {
			missing = append(missing, name)
			continue
		}
		verdict := FalsifyPinsNothing
		switch {
		case redAtHead[name] && passed:
			verdict = FalsifyWitness
		case redAtHead[name]:
			verdict = FalsifyWrongReason
		case !passed:
			verdict = FalsifyInverted
		}
		verdicts = append(verdicts, FalsifyVerdict{Test: name, Verdict: verdict})
	}
	sort.Slice(verdicts, func(i, j int) bool { return verdicts[i].Test < verdicts[j].Test })
	var notes []string
	if len(missing) > 0 {
		sort.Strings(missing)
		notes = append(notes, "not run with the candidate fix: "+strings.Join(missing, ", "))
	}
	if len(verdicts) == 0 {
		return nil, notes, fmt.Errorf("no changed test ran with the candidate fix: %s", tail(out, 600))
	}
	return verdicts, notes, nil
}

// packageTestCovers reports whether a test name belongs to a package redcheck named as code HEAD lacks.
func packageTestCovers(pkg, name string) bool {
	if pkg == "." {
		return true
	}
	return strings.Contains(name, "/"+pkg+".") || strings.HasPrefix(name, pkg+".")
}

// falsifyRestore puts exactly the saved source files back at HEAD: tracked ones through git, new ones by
// removing them. It names no test file, so the agent's tests survive. What it left behind is no claim of its
// own: the caller proves the restore against the whole working-tree status.
func falsifyRestore(root string, change falsifyChange) (restored []string, err error) {
	var tracked, fresh []string
	for _, rel := range change.sourceFiles {
		if _, inHead := change.headModes[rel]; inHead {
			tracked = append(tracked, rel)
		} else {
			fresh = append(fresh, rel)
		}
	}
	if len(tracked) > 0 {
		out, e := gitFalsify(root, append([]string{"restore", "--source=HEAD", "--staged", "--worktree", "--"}, tracked...)...)
		if e != nil {
			return restored, fmt.Errorf("git restore %s: %v: %s", strings.Join(tracked, ", "), e, strings.TrimSpace(out))
		}
		restored = append(restored, tracked...)
	}
	for _, rel := range fresh {
		if !change.untracked[rel] {
			// A staged new file lives in the index too, and status would keep reporting it.
			if out, e := gitFalsify(root, "rm", "--cached", "--ignore-unmatch", "-q", "--", rel); e != nil {
				return restored, fmt.Errorf("unstage the new source file %s: %v: %s", rel, e, strings.TrimSpace(out))
			}
		}
		if e := os.Remove(filepath.Join(root, filepath.FromSlash(rel))); e != nil && !os.IsNotExist(e) {
			return restored, fmt.Errorf("remove the new source file %s: %v", rel, e)
		}
		restored = append(restored, rel)
	}
	sort.Strings(restored)
	return restored, nil
}

// falsifyStatus is the working tree's status as its sorted "XY path" entries, the whole of it. Two readings
// are what proves a restore: a file it rewrote outside the source set cannot pass unnoticed.
func falsifyStatus(root string) ([]string, error) {
	out, err := gitFalsify(root, "status", "--porcelain", "-z", "--no-renames")
	if err != nil {
		return nil, fmt.Errorf("git status: %v: %s", err, strings.TrimSpace(out))
	}
	var entries []string
	for _, entry := range splitNul(out) {
		if len(entry) > 3 {
			entries = append(entries, entry)
		}
	}
	sort.Strings(entries)
	return entries, nil
}

// falsifySourceLeft names the source files git status still reports: the ones a failed or partial restore left
// behind. The list comes from the tree, never from the files the command meant to restore.
func falsifySourceLeft(entries, sourceFiles []string) []string {
	want := setOf(sourceFiles)
	seen := map[string]bool{}
	var left []string
	for _, entry := range entries {
		if p := entry[3:]; want[p] && !seen[p] {
			seen[p] = true
			left = append(left, p)
		}
	}
	sort.Strings(left)
	return left
}

// falsifyStillChanged names what a run that never restored anything left in the tree, for the refusals and the
// failed saves; a status it cannot read leaves the answer empty rather than invented.
func falsifyStillChanged(root string, sourceFiles []string) []string {
	entries, err := falsifyStatus(root)
	if err != nil {
		return nil
	}
	return falsifySourceLeft(entries, sourceFiles)
}

// falsifyOutside names the paths whose status differs between two readings and that are not in allowed: the
// files a restore touched although it promised to touch only the ones it saved.
func falsifyOutside(before, after []string, allowed map[string]bool) []string {
	was, is := map[string]bool{}, map[string]bool{}
	for _, entry := range before {
		was[entry] = true
	}
	for _, entry := range after {
		is[entry] = true
	}
	changed := map[string]bool{}
	for _, entry := range append(append([]string{}, before...), after...) {
		p := entry[3:]
		if allowed[p] || changed[p] {
			continue
		}
		if was[entry] != is[entry] {
			changed[p] = true
		}
	}
	var out []string
	for p := range changed {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func setOf(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, v := range values {
		set[v] = true
	}
	return set
}

func splitNul(out string) []string {
	var values []string
	for _, v := range strings.Split(out, "\x00") {
		if v != "" {
			values = append(values, v)
		}
	}
	return values
}

// gitFalsify runs git with GIT_LITERAL_PATHSPECS=1. Every path this command hands to git is a file name it read
// from git, not a pattern: without the flag `git restore -- 'a*.go'` also restores agent_test.go, and a source
// file named after a glob would eat the tests this command promises never to touch.
func gitFalsify(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_LITERAL_PATHSPECS=1")
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// gitFalsifyBytes runs git and returns stdout alone, because a binary diff cannot share its stream with
// diagnostics. git diff --no-index exits 1 when the files differ, which is the answer, not a failure.
func gitFalsifyBytes(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_LITERAL_PATHSPECS=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			return stdout.Bytes(), nil
		}
		return nil, fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}
