// Package check answers the testing questions from the repository alone: no hook payload, no
// transcript, no host. Claude Code, another agent, a Makefile and CI can all run it, because the
// only thing it needs is git, the tests, and a plan when one was written.
package check

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alesierraalta/tsp/internal/bench"
	"github.com/alesierraalta/tsp/internal/gate"
	"github.com/alesierraalta/tsp/internal/plan"
)

// Deps are the process boundaries, injected so the decision is testable without a repository.
type Deps struct {
	Git      func(dir string, args ...string) (string, error)
	ReadFile func(path string) (string, error)
	PlanPath string
	// Suite is the command that runs the changed tests; empty lets the red check infer it.
	Suite string
	// RedCheck runs the tests changed since HEAD against HEAD's code.
	RedCheck func(root string) (Witness, error)
	// Waive, when not empty, records that the current change needs no new test, with this reason.
	Waive string
	// Base names a commit-ish to check the range against instead of the working tree: in CI the
	// candidate is `base...HEAD`, and the tree there holds HEAD, so a status reading would answer
	// "nothing is changed" for every pull request. Empty reads the working tree as always.
	Base string
	// Paths, when not empty, answers for those paths alone: whether the production source this work
	// touched still owes a test. The plan's remaining targets never reach that answer, because how much
	// of a run's agenda is open is progress, not this change's debt. One path per element, never split
	// on commas.
	Paths []string
	// Exists reports whether a --paths name is present in the working tree, resolved against the
	// repository root. Injected so the scoped guard is testable without a filesystem; Run defaults it
	// to an os.Lstat under the root, so a tracked symlink counts as present whatever it points at.
	Exists func(root, rel string) bool
}

// WaiverName is the file a no-test-needed waiver is recorded in, at the repository root.
const WaiverName = ".tsp-waiver.json"

// Witness is what the red check saw: the changed test files, the new tests that fail on HEAD's code
// (they pin a defect), and those that pass there (they pin nothing).
type Witness struct {
	TestFiles   []string
	Red         []string
	PinsNothing []string
}

const optOutName = ".no-testing-gate"

// Result is what the caller prints and exits with.
type Result struct {
	Exit  int
	Text  string
	Files []string
}

// MaxNamed is how many changed files the text names before counting the rest.
const MaxNamed = 6

// Run reports what the repository at cwd still owes. Exit 1 means there is something to do;
// exit 0 means there is not, and the text says which of the two it is either way.
func Run(cwd string, d Deps) Result {
	if d.Git == nil {
		d.Git = gitAt
	}
	if d.ReadFile == nil {
		d.ReadFile = readFile
	}
	if d.Exists == nil {
		d.Exists = func(root, rel string) bool { _, err := os.Lstat(filepath.Join(root, rel)); return err == nil }
	}
	rootOut, err := d.Git(cwd, "rev-parse", "--show-toplevel")
	root := strings.TrimSpace(rootOut)
	if err != nil || root == "" {
		return Result{Text: "not a git repository: nothing to check"}
	}
	planRelPath := d.PlanPath
	// declaredRun scopes the verdict to the run the declaration selected; it stays empty when the
	// caller named a path explicitly, and manual check keeps its legacy whole-plan reading then.
	declaredRun := ""
	if planRelPath != "" {
		if err := plan.ValidatePlanPath("--path", planRelPath); err != nil {
			return Result{Exit: 1, Text: err.Error()}
		}
		planRelPath = filepath.Clean(planRelPath)
	} else {
		var cfgErr error
		planRelPath, declaredRun, cfgErr = plan.Resolve(root, d.ReadFile)
		if cfgErr != nil {
			return Result{Exit: 1, Text: "the plan declaration could not be read: " + cfgErr.Error()}
		}
	}
	planFilePath := filepath.Join(root, planRelPath)
	// The plan is read before the diff so the versioning caveat rides with every verdict, including
	// the quiet ones: a plan git will never version is a half persistence whatever the tree shows.
	body, planErr := d.ReadFile(planFilePath)
	warn := ""
	if planErr == nil {
		warn = ignoredPlanWarning(root, planRelPath, d)
	}
	if d.Base != "" && (d.Waive != "" || len(d.Paths) > 0) {
		return Result{Exit: 2, Text: warn + "--base checks a commit range: --no-test-needed and --paths answer for the working tree and cannot be combined with it"}
	}
	statusOut, all, err := d.changedPaths(root)
	if err != nil {
		return Result{Text: warn + err.Error()}
	}
	scope := ""
	if d.Base != "" {
		scope = "base " + d.Base + ": "
	}
	var files []string
	seen := map[string]bool{}
	for _, p := range all {
		if gate.IsProductionSource(p) && !seen[p] {
			seen[p] = true
			files = append(files, p)
		}
	}
	if len(files) == 0 {
		return Result{Text: warn + scope + "no production source is changed: nothing to check"}
	}
	if d.Waive != "" && planErr == nil {
		return Result{Exit: 2, Files: files, Text: warn + "--no-test-needed applies only without a plan: " + planRelPath + " is this change's record"}
	}
	if len(d.Paths) > 0 {
		res := scopedVerdict(cwd, root, files, all, deletedPaths(statusOut), d)
		res.Text = warn + res.Text
		return res
	}
	if planErr != nil && d.Waive != "" {
		res := recordWaiver(root, files, d)
		res.Files = files
		res.Text = warn + scope + changedLine(files) + "\n" + res.Text
		return res
	}
	if planErr != nil {
		if d.RedCheck == nil {
			suite, base, candidate := d.Suite, d.Base, all
			d.RedCheck = func(root string) (Witness, error) { return redCheck(root, suite, base, candidate) }
		}
		res := witnessVerdict(root, files, d)
		res.Files = files
		res.Text = warn + scope + changedLine(files) + "\n" + res.Text
		return res
	}
	// The verdict is scoped to the declared run when one is declared: GapsForRun counts only the
	// rows that run owns, so another run's pending rows are never this run's debt and another run's
	// finished rows are never this run's proof of completion. With no run declared the run is empty
	// and every row counts, the legacy whole-plan reading the manual command has always had.
	gaps, err := plan.GapsForRun(body, declaredRun)
	if err != nil {
		return Result{Exit: 1, Files: files, Text: warn + scope + changedLine(files) + "\nthe plan could not be read: " + err.Error()}
	}
	if !gaps.Any() && gaps.Micro != "" {
		return Result{Files: files, Text: warn + scope + changedLine(files) + "\n" + planRelPath + " owes nothing: it is a micro plan for one small function, which owes no layer sweep"}
	}
	if !gaps.Any() {
		owes := planRelPath + " owes nothing"
		if declaredRun != "" {
			owes += " for run " + declaredRun
		}
		return Result{Files: files, Text: warn + scope + changedLine(files) + "\n" + owes + ": every assigned layer was swept and every ranked target is done"}
	}
	return Result{Exit: 1, Files: files, Text: warn + scope + changedLine(files) + "\n" + gaps.Report()}
}

// changedPaths reads the candidate: the commit range when Base names one, the working tree otherwise.
// It returns the porcelain reading for the callers that need deletions, and the changed names.
func (d Deps) changedPaths(root string) (statusOut string, names []string, err error) {
	if d.Base != "" {
		out, diffErr := d.Git(root, "diff", "--name-only", "-z", "--no-renames", d.Base+"...HEAD")
		if diffErr != nil {
			return "", nil, errors.New("git diff failed for --base " + d.Base + ": nothing to check")
		}
		return "", splitNUL(out), nil
	}
	statusOut, statusErr := d.Git(root, "status", "--porcelain", "-z", "-uall")
	if statusErr != nil {
		return "", nil, errors.New("git status failed: nothing to check")
	}
	return statusOut, gate.ParsePorcelain(statusOut), nil
}

// splitNUL reads a -z listing: git separates the names with NUL bytes and never quotes them.
func splitNUL(out string) []string {
	var names []string
	for _, p := range strings.Split(out, "\x00") {
		if p != "" {
			names = append(names, p)
		}
	}
	return names
}

// noWitnessDebt is what a change owes when no test came with it: one sentence, because it reaches the
// model as an instruction and the human as the reason nothing was verified.
const noWitnessDebt = "no test was added or changed with it: run the testing discipline (\"haz el TSP\"), record tsp check --no-test-needed \"<reason>\" when this change warrants no test, or add " + optOutName + " when this repository does not warrant it"

// scopedVerdict answers for the named paths only: what they owe a test. It reads the tree, so a test
// file changed with the source settles the debt without re-running anything, and the plan is never
// consulted: how much of a run's agenda is still open is progress, not this change's debt. A caller
// names its paths from the directory it asked about, and a path outside the repository is refused
// rather than answered, because "it did not change" would read as "this change owes nothing". A name
// that is neither a file nor a changed path is refused the same way: it is a mistake, and answering
// "nothing to check" for it would hide the debt the caller asked about.
func scopedVerdict(cwd, root string, changed, all, deleted []string, d Deps) Result {
	// The caller spells its paths from the directory it asked about, and --cwd keeps its default "."
	// there: a relative value is relative to where the process stands, never to the repository root.
	// Making it absolute first is what keeps a caller standing in a subdirectory from having its paths
	// read as root-relative, which would answer "nothing to check" for a file it just named. A --cwd
	// no prefix can be read from is refused, never answered from the root in its place.
	prefix, placed := repositoryPrefix(cwd, root)
	if !placed {
		return Result{Exit: 2, Text: cwd + ": --cwd must be a directory inside the repository at " + root}
	}
	// A deleted file is still part of this change: git status carries its name even though nothing is
	// left on disk to stat, and ParsePorcelain drops deletions on purpose, so they are read here.
	inStatus := map[string]bool{}
	for _, p := range append(append([]string(nil), all...), deleted...) {
		inStatus[filepath.ToSlash(filepath.Clean(p))] = true
	}
	named := make(map[string]bool, len(d.Paths))
	for _, p := range d.Paths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		given := p
		if !filepath.IsAbs(p) {
			p = filepath.Join(prefix, p)
		} else {
			p = repositoryRelative(root, p)
		}
		if outside(p) {
			return Result{Exit: 2, Text: given + ": a named path must be inside the repository at " + root}
		}
		clean := filepath.ToSlash(filepath.Clean(p))
		if !d.Exists(root, p) && !inStatus[clean] {
			return Result{Exit: 2, Text: "check: --paths names " + given + ", which is neither a file nor a changed path"}
		}
		named[clean] = true
	}
	var in []string
	for _, p := range changed {
		if named[p] {
			in = append(in, p)
		}
	}
	if len(in) == 0 {
		return Result{Text: "no production source in the named paths is changed: nothing to check"}
	}
	sort.Strings(in)
	line := changedLine(in)
	if _, err := d.ReadFile(filepath.Join(root, optOutName)); err == nil {
		return Result{Files: in, Text: line + "\n" + optOutName + " is present: this repository opted out of the testing check"}
	}
	if reason, ok := waived(root, in, d); ok {
		return Result{Files: in, Text: line + "\nthis change needs no new test (waived: " + reason + ")"}
	}
	var tests []string
	for _, p := range all {
		// The rule is the bench's, not a copy: this file already depends on internal/bench for
		// RedCheck, and a second definition would let the two answers drift apart.
		if bench.IsTestFile(p) {
			tests = append(tests, p)
		}
	}
	if len(tests) == 0 {
		return Result{Exit: 1, Files: in, Text: line + "\n" + noWitnessDebt}
	}
	sort.Strings(tests)
	return Result{Files: in, Text: line + "\n" + "a test was added or changed with it: " + capNames(tests)}
}

// outside reports a repository-relative path that leaves the repository.
func outside(rel string) bool {
	rel = filepath.Clean(rel)
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel)
}

// repositoryRelative is an absolute --paths value as a repository-relative path, read the way git
// reads the root: physically. A host passes the path its editor reported, which may be spelled
// through a link that reached the repository, while git reports the root with every link resolved;
// compared as spelled, a file inside the repository reads as outside it and is refused. Only the
// directory is resolved: the named file itself may be a tracked symlink, and following it would
// answer for its target instead. The directory may be gone with a deleted file, so its nearest
// existing ancestor is resolved and the rest kept as written. A root that cannot be resolved
// leaves the lexical reading, which is all a path that names nothing on disk can be compared by.
func repositoryRelative(root, abs string) string {
	abs = filepath.Clean(abs)
	lexical := func() string {
		if rel, err := filepath.Rel(root, abs); err == nil {
			return rel
		}
		return abs
	}
	base, err := filepath.EvalSymlinks(root)
	if err != nil {
		return lexical()
	}
	dir, rest := filepath.Dir(abs), filepath.Base(abs)
	for {
		if real, err := filepath.EvalSymlinks(dir); err == nil {
			if rel, err := filepath.Rel(base, filepath.Join(real, rest)); err == nil {
				return rel
			}
			return lexical()
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return lexical()
		}
		rest = filepath.Join(filepath.Base(dir), rest)
		dir = parent
	}
}

// repositoryPrefix is --cwd as a repository-relative directory, read the way git reads the root and the
// changed files it reports. Both sides are resolved through their links because git reports physical
// paths: a --cwd reached through a symlink, kept under the link's own spelling, prefixes the named
// paths with a place git never reported, so the file the caller named answers "nothing to check". A
// --cwd that is not a directory inside the repository — outside it, absent, or a file — earns the
// refusal its caller returns instead: with no prefix, the named paths are read root-relative, which
// accuses a file the caller never named or clears the one it did. A root that cannot itself be
// resolved leaves no physical root to read --cwd against, and the lexical reading stands.
func repositoryPrefix(cwd, root string) (string, bool) {
	abs, err := filepath.Abs(filepath.Clean(cwd))
	if err != nil {
		return "", false
	}
	base, err := filepath.EvalSymlinks(root)
	if err != nil {
		rel, err := filepath.Rel(root, abs)
		if err != nil || outside(rel) {
			return "", true
		}
		return rel, true
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", false
	}
	if info, err := os.Stat(real); err != nil || !info.IsDir() {
		return "", false
	}
	rel, err := filepath.Rel(base, real)
	if err != nil || outside(rel) {
		return "", false
	}
	return rel, true
}

// deletedPaths reads the names `git status --porcelain -z -uall` reports as gone from the tree,
// which gate.ParsePorcelain drops: deletions, and the source of a rename or copy, which -z prints as
// the field after its entry. Such a name has nothing on disk, but a --paths value that spells one
// names part of this change, not a mistyped path.
func deletedPaths(raw string) []string {
	var out []string
	fields := strings.Split(raw, "\x00")
	for i := 0; i < len(fields); i++ {
		entry := fields[i]
		if len(entry) < 4 {
			continue
		}
		if strings.ContainsRune(entry[:2], 'D') {
			out = append(out, entry[3:])
		}
		if strings.ContainsAny(entry[:2], "RC") && i+1 < len(fields) {
			i++
			out = append(out, fields[i])
		}
	}
	return out
}

// witnessVerdict is the reading without a plan: test-strategy 0.7 writes none, so the tests are what the
// tree can corroborate. A change is settled when at least one new test fails on HEAD's code, or when the
// repository opted out with .no-testing-gate. A suite that cannot be judged fails closed.
func witnessVerdict(root string, files []string, d Deps) Result {
	if _, err := d.ReadFile(filepath.Join(root, optOutName)); err == nil {
		return Result{Text: optOutName + " is present: this repository opted out of the testing check"}
	}
	if reason, ok := waived(root, files, d); ok {
		return Result{Text: "this change needs no new test (waived: " + reason + ")"}
	}
	w, err := d.RedCheck(root)
	if err != nil {
		return Result{Exit: 1, Text: "the new tests could not be judged: " + err.Error()}
	}
	if len(w.TestFiles) == 0 {
		return Result{Exit: 1, Text: noWitnessDebt}
	}
	if len(w.Red) == 0 {
		return Result{Exit: 1, Text: "no new test fails on HEAD's code, so none of them pins a defect: change the input or setup until a test fails for the reason the contract names, or run tsp redcheck to see each test"}
	}
	return Result{Text: fmt.Sprintf("%d new test(s) fail on HEAD's code: %s", len(w.Red), strings.Join(w.Red, ", "))}
}

// waiver is what --no-test-needed records: a person's reason, bound to the content of the changed
// production files, so code written after it is not covered.
type waiver struct {
	Fingerprint string   `json:"fingerprint"`
	Files       []string `json:"files"`
	Reason      string   `json:"reason"`
	Recorded    string   `json:"recorded"`
}

// changeFingerprint hashes each changed production file's path and content, in path order.
func changeFingerprint(root string, files []string, d Deps) string {
	sorted := append([]string(nil), files...)
	sort.Strings(sorted)
	h := sha256.New()
	for _, f := range sorted {
		body, err := d.ReadFile(filepath.Join(root, f))
		if err != nil {
			body = "\x00unreadable"
		}
		sum := sha256.Sum256([]byte(body))
		fmt.Fprintf(h, "%s\x00%x\n", f, sum)
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

func recordWaiver(root string, files []string, d Deps) Result {
	w := waiver{Fingerprint: changeFingerprint(root, files, d), Files: files, Reason: d.Waive, Recorded: time.Now().UTC().Format(time.RFC3339)}
	body, err := json.MarshalIndent(w, "", "  ")
	if err == nil {
		err = os.WriteFile(filepath.Join(root, WaiverName), append(body, '\n'), 0o644)
	}
	if err != nil {
		return Result{Exit: 1, Text: "the waiver could not be recorded: " + err.Error()}
	}
	return Result{Text: "recorded in " + WaiverName + ": " + d.Waive + "\ncommit it with the change so the reviewer sees why there is no test; editing the code again voids it"}
}

// waived reports whether the recorded waiver covers exactly the change in the tree now.
func waived(root string, files []string, d Deps) (string, bool) {
	raw, err := d.ReadFile(filepath.Join(root, WaiverName))
	if err != nil {
		return "", false
	}
	var w waiver
	if json.Unmarshal([]byte(raw), &w) != nil || strings.TrimSpace(w.Reason) == "" {
		return "", false
	}
	return w.Reason, w.Fingerprint == changeFingerprint(root, files, d)
}

// redCheck judges the candidate's tests against the code the change started from: base's tree when the
// caller named a range, HEAD's tree otherwise.
func redCheck(root, suite, base string, candidate []string) (Witness, error) {
	var rep bench.RedCheckReport
	var err error
	if base != "" {
		rep, err = bench.RedCheckRange(root, suite, base, candidate, 5*time.Minute)
	} else {
		rep, err = bench.RedCheck(root, suite, 5*time.Minute)
	}
	return Witness{TestFiles: rep.TestFiles, Red: rep.Red, PinsNothing: rep.PinsNothing}, err
}

// ignoredPlanWarning names the one thing check cannot see by reading the file: a plan path git will
// never version, so the run's central artifact can be lost while the repository reads as settled.
// It is a warning, never a verdict: check's exit code decides whether a run is blocked.
func ignoredPlanWarning(root, path string, d Deps) string {
	if _, err := d.Git(root, "check-ignore", "-q", path); err == nil {
		return "warning: git ignores " + path + ": the run's evidence cannot be versioned\n"
	}
	return ""
}

func changedLine(files []string) string {
	return fmt.Sprintf("%d production source file(s) changed: %s", len(files), capNames(files))
}

// capNames lists at most MaxNamed paths and counts the rest: a host reads the answer through a bounded pipe.
func capNames(paths []string) string {
	if len(paths) <= MaxNamed {
		return strings.Join(paths, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(paths[:MaxNamed], ", "), len(paths)-MaxNamed)
}

func gitAt(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return string(out), err
}

func readFile(path string) (string, error) {
	body, err := os.ReadFile(path)
	return string(body), err
}
