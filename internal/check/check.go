// Package check answers the testing questions from the repository alone: no hook payload, no
// transcript, no host. Claude Code, another agent, a Makefile and CI can all run it, because the
// only thing it needs is git, the tests, and a plan when one was written.
package check

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	if d.RedCheck == nil {
		suite := d.Suite
		d.RedCheck = func(root string) (Witness, error) { return redCheck(root, suite) }
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
	statusOut, err := d.Git(root, "status", "--porcelain", "-z", "-uall")
	if err != nil {
		return Result{Text: warn + "git status failed: nothing to check"}
	}
	var files []string
	seen := map[string]bool{}
	for _, p := range gate.ParsePorcelain(statusOut) {
		if gate.IsProductionSource(p) && !seen[p] {
			seen[p] = true
			files = append(files, p)
		}
	}
	if len(files) == 0 {
		return Result{Text: warn + "no production source is changed: nothing to check"}
	}
	if d.Waive != "" && planErr == nil {
		return Result{Exit: 2, Files: files, Text: warn + "--no-test-needed applies only without a plan: " + planRelPath + " is this change's record"}
	}
	if planErr != nil && d.Waive != "" {
		res := recordWaiver(root, files, d)
		res.Files = files
		res.Text = warn + changedLine(files) + "\n" + res.Text
		return res
	}
	if planErr != nil {
		res := witnessVerdict(root, files, d)
		res.Files = files
		res.Text = warn + changedLine(files) + "\n" + res.Text
		return res
	}
	// The verdict is scoped to the declared run when one is declared: GapsForRun counts only the
	// rows that run owns, so another run's pending rows are never this run's debt and another run's
	// finished rows are never this run's proof of completion. With no run declared the run is empty
	// and every row counts, the legacy whole-plan reading the manual command has always had.
	gaps, err := plan.GapsForRun(body, declaredRun)
	if err != nil {
		return Result{Exit: 1, Files: files, Text: warn + changedLine(files) + "\nthe plan could not be read: " + err.Error()}
	}
	if !gaps.Any() && gaps.Micro != "" {
		return Result{Files: files, Text: warn + changedLine(files) + "\n" + planRelPath + " owes nothing: it is a micro plan for one small function, which owes no layer sweep"}
	}
	if !gaps.Any() {
		owes := planRelPath + " owes nothing"
		if declaredRun != "" {
			owes += " for run " + declaredRun
		}
		return Result{Files: files, Text: warn + changedLine(files) + "\n" + owes + ": every assigned layer was swept and every ranked target is done"}
	}
	return Result{Exit: 1, Files: files, Text: warn + changedLine(files) + "\n" + gaps.Report()}
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
		return Result{Exit: 1, Text: "no test was added or changed with it: run the testing discipline (\"haz el TSP\"), record tsp check --no-test-needed \"<reason>\" when this change warrants no test, or add " + optOutName + " when this repository does not warrant it"}
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

func redCheck(root, suite string) (Witness, error) {
	rep, err := bench.RedCheck(root, suite, 5*time.Minute)
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
	shown := files
	extra := ""
	if len(files) > MaxNamed {
		shown = files[:MaxNamed]
		extra = fmt.Sprintf(" and %d more", len(files)-MaxNamed)
	}
	return fmt.Sprintf("%d production source file(s) changed: %s%s", len(files), strings.Join(shown, ", "), extra)
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
