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
	testFiles, sourceFiles, untracked, headFiles, err := falsifyChanges(root)
	if err != nil {
		return rep, err
	}
	rep.TestFiles, rep.SourceFiles = testFiles, sourceFiles
	if len(sourceFiles) == 0 {
		return rep, errors.New("no source change against HEAD: write the candidate fix in the source first")
	}
	untrackedSet := setOf(untracked)
	patch, err := falsifyPatch(root, sourceFiles, untrackedSet)
	if err != nil {
		rep.Left = sourceFiles
		return rep, fmt.Errorf("read the candidate fix: %v; nothing was restored, the source still differs from HEAD", err)
	}
	rep.Patch, err = falsifySavePatch(root, patch)
	if err != nil {
		rep.Left = sourceFiles
		return rep, fmt.Errorf("save the candidate fix to %s: %v; nothing was restored, %d source file(s) still differ from HEAD: %s",
			rep.Patch, err, len(sourceFiles), strings.Join(sourceFiles, ", "))
	}
	var verdicts []FalsifyVerdict
	var notes []string
	var judgeErr error
	if len(testFiles) > 0 {
		red, before, after, _, redErr := redCheckDetail(root, suite, "HEAD", testFiles, timeout)
		rep.Suite, rep.Uncollected = red.Suite, red.Uncollected
		if redErr != nil {
			judgeErr = redErr
		} else {
			verdicts, notes, judgeErr = falsifyVerdicts(root, testFiles, red, before, after, timeout)
		}
	}
	restored, left, err := falsifyRestore(root, sourceFiles, headFiles, untrackedSet)
	rep.Restored, rep.Left, rep.Notes, rep.Verdicts = restored, left, notes, verdicts
	if err != nil {
		return rep, fmt.Errorf("restore the source: %v; saved candidate fix: %s; still changed: %s", err, rep.Patch, strings.Join(left, ", "))
	}
	if judgeErr != nil {
		return rep, judgeErr
	}
	if len(testFiles) == 0 {
		return rep, nil
	}
	if len(verdicts) == 0 {
		return rep, fmt.Errorf("no changed test could be judged with the candidate fix; saved candidate fix: %s", rep.Patch)
	}
	return rep, nil
}

func falsifyRoot(dir string) (string, error) {
	out, err := gitRun(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("not inside a git repository: %s", strings.TrimSpace(out))
	}
	return strings.TrimSpace(out), nil
}

// falsifyChanges splits the working-tree change against HEAD into test files and source files. A deleted file
// is still part of the change, and an untracked file is the change too when git does not ignore it.
func falsifyChanges(root string) (testFiles, sourceFiles, untracked []string, headFiles map[string]bool, err error) {
	trackedOut, err := gitRun(root, "diff", "--name-only", "--no-renames", "-z", "HEAD")
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("read the working-tree change: %v: %s", err, strings.TrimSpace(trackedOut))
	}
	untrackedOut, err := gitRun(root, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("list the untracked files: %v: %s", err, strings.TrimSpace(untrackedOut))
	}
	headOut, err := gitRun(root, "ls-tree", "-r", "--name-only", "-z", "HEAD")
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("read HEAD's files: %v: %s", err, strings.TrimSpace(headOut))
	}
	headFiles = map[string]bool{}
	for _, p := range splitNul(headOut) {
		headFiles[p] = true
	}
	untracked = splitNul(untrackedOut)
	seen := map[string]bool{}
	for _, p := range append(splitNul(trackedOut), untracked...) {
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		if isTestFile(p) {
			testFiles = append(testFiles, p)
		} else {
			sourceFiles = append(sourceFiles, p)
		}
	}
	sort.Strings(testFiles)
	sort.Strings(sourceFiles)
	sort.Strings(untracked)
	return testFiles, sourceFiles, untracked, headFiles, nil
}

// falsifyPatch is the candidate fix as one binary-safe patch: `git diff HEAD` for everything git tracks,
// including a staged new file, plus a no-index diff for each new source file git has never seen.
func falsifyPatch(root string, sourceFiles []string, untracked map[string]bool) ([]byte, error) {
	var tracked, fresh []string
	for _, rel := range sourceFiles {
		if untracked[rel] {
			fresh = append(fresh, rel)
		} else {
			tracked = append(tracked, rel)
		}
	}
	var patch bytes.Buffer
	if len(tracked) > 0 {
		out, err := gitBytes(root, append([]string{"diff", "--binary", "--no-renames", "HEAD", "--"}, tracked...)...)
		if err != nil {
			return nil, err
		}
		patch.Write(out)
	}
	for _, rel := range fresh {
		out, err := gitBytes(root, "diff", "--no-index", "--binary", "--", "/dev/null", rel)
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

// falsifySavePatch writes the patch under <git-dir>/tsp/falsify with a UTC timestamp. The git dir is resolved
// by git, so a linked worktree saves into its own git dir instead of a .git beside the sources.
func falsifySavePatch(root string, patch []byte) (string, error) {
	out, err := gitRun(root, "rev-parse", "--git-dir")
	if err != nil {
		return "", fmt.Errorf("locate the git dir: %v: %s", err, strings.TrimSpace(out))
	}
	gitDir := strings.TrimSpace(out)
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(root, gitDir)
	}
	name := time.Now().UTC().Format("20060102T150405.000000000Z") + ".patch"
	path := filepath.Join(gitDir, "tsp", "falsify", name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return path, err
	}
	if err := os.WriteFile(path, patch, 0o644); err != nil {
		return path, err
	}
	return path, nil
}

// falsifyVerdicts runs the same changed tests on the working tree, which carries the candidate fix, and
// compares each outcome with the one HEAD's code gave.
func falsifyVerdicts(root string, testFiles []string, red RedCheckReport, before, after map[string]bool, timeout time.Duration) ([]FalsifyVerdict, []string, error) {
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
// removing them. It then proves with git status that no source file still differs. Test files are never named
// here, so the agent's tests survive.
func falsifyRestore(root string, sourceFiles []string, headFiles, untracked map[string]bool) (restored, left []string, err error) {
	var tracked, fresh []string
	for _, rel := range sourceFiles {
		if headFiles[rel] {
			tracked = append(tracked, rel)
		} else {
			fresh = append(fresh, rel)
		}
	}
	if len(tracked) > 0 {
		out, e := gitRun(root, append([]string{"restore", "--source=HEAD", "--staged", "--worktree", "--"}, tracked...)...)
		if e != nil {
			return restored, sourceFiles, fmt.Errorf("git restore %s: %v: %s", strings.Join(tracked, ", "), e, strings.TrimSpace(out))
		}
		restored = append(restored, tracked...)
	}
	for _, rel := range fresh {
		if !untracked[rel] {
			// A staged new file lives in the index too, and status would keep reporting it.
			if out, e := gitRun(root, "rm", "--cached", "--ignore-unmatch", "-q", "--", rel); e != nil {
				return restored, remaining(sourceFiles, restored), fmt.Errorf("unstage the new source file %s: %v: %s", rel, e, strings.TrimSpace(out))
			}
		}
		if e := os.Remove(filepath.Join(root, filepath.FromSlash(rel))); e != nil && !os.IsNotExist(e) {
			return restored, remaining(sourceFiles, restored), fmt.Errorf("remove the new source file %s: %v", rel, e)
		}
		restored = append(restored, rel)
	}
	sort.Strings(restored)
	left, err = falsifyLeftovers(root, sourceFiles)
	if err != nil {
		return restored, nil, err
	}
	if len(left) > 0 {
		return restored, left, fmt.Errorf("the working tree still differs from HEAD in %s", strings.Join(left, ", "))
	}
	return restored, nil, nil
}

// falsifyLeftovers is the proof that the restore is complete: the source files git status still reports.
func falsifyLeftovers(root string, sourceFiles []string) ([]string, error) {
	want := setOf(sourceFiles)
	out, err := gitRun(root, "status", "--porcelain", "-z", "--no-renames")
	if err != nil {
		return nil, fmt.Errorf("prove the restore with git status: %v: %s", err, strings.TrimSpace(out))
	}
	var left []string
	for _, entry := range strings.Split(out, "\x00") {
		if len(entry) < 4 {
			continue
		}
		if p := entry[3:]; want[p] {
			left = append(left, p)
		}
	}
	sort.Strings(left)
	return left, nil
}

func remaining(files, done []string) []string {
	seen := setOf(done)
	var out []string
	for _, f := range files {
		if !seen[f] {
			out = append(out, f)
		}
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

// gitBytes runs git and returns stdout alone, because a binary diff cannot share its stream with diagnostics.
// git diff --no-index exits 1 when the files differ, which is the answer, not a failure.
func gitBytes(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
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
