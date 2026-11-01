package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alesierraalta/tsp/internal/check"
	"github.com/alesierraalta/tsp/internal/findings"
)

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// committedUnit writes the files, commits them, and returns the repo: this is the shape a turn leaves
// when it commits its work unit before it ends.
func committedUnit(t *testing.T, body map[string]string) string {
	t.Helper()
	dir := pathsRepo(t, "a.go")
	for name, text := range body {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0o755); err != nil && filepath.Dir(name) != "." {
			t.Fatal(err)
		}
		writeIn(t, dir, name, text)
	}
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-qm", "work unit")
	return dir
}

type verdict struct {
	Decision string `json:"decision"`
	Debt     string `json:"debt"`
	Reason   string `json:"reason"`
	Exit     int    `json:"exit"`
}

func readVerdict(t *testing.T, out string) verdict {
	t.Helper()
	var v verdict
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &v); err != nil {
		t.Fatalf("the verdict must be one JSON object: %v in %q", err, out)
	}
	return v
}

// A work unit committed before the turn ended leaves a clean tree. Reading the tree alone answered
// "nothing is changed" for the very source the commit changed, so the offer went silent after every
// commit; --since is what reads the unit again.
func TestCheckSinceReadsAWorkUnitThatIsAlreadyCommitted(t *testing.T) {
	t.Setenv("TSP_GENTLE_OBSERVATION", "")
	dir := committedUnit(t, map[string]string{"a.go": "package sample\n\nfunc F() int { return 1 }\n"})

	if code, out, errOut := runCheckCapturing(t, "--cwd", dir, "--paths", "a.go", "--json"); code != 0 || !strings.Contains(out, `"decision":"none"`) {
		t.Fatalf("the clean tree alone must stay quiet: code=%d stdout=%q stderr=%q", code, out, errOut)
	}

	code, out, errOut := runCheckCapturing(t, "--cwd", dir, "--since", "HEAD~1", "--paths", "a.go", "--json")
	if code != 1 {
		t.Fatalf("the committed unit owes a test: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	v := readVerdict(t, out)
	if v.Decision != "ask" || v.Exit != 1 {
		t.Fatalf("a standalone caller asks: %+v", v)
	}
	if !strings.Contains(v.Reason, "no gate is declared") {
		t.Fatalf("the reason must say what decided: %q", v.Reason)
	}
}

// The same unit in a Gentle session is required: the harness runs, nobody is asked.
func TestCheckSinceRequiresTheRunInAGentleSession(t *testing.T) {
	t.Setenv("TSP_GENTLE_OBSERVATION", "pi-session-gentle-active")
	dir := committedUnit(t, map[string]string{"a.go": "package sample\n\nfunc F() int { return 1 }\n"})

	code, out, errOut := runCheckCapturing(t, "--cwd", dir, "--since", "HEAD~1", "--paths", "a.go", "--json")
	if code != 1 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	v := readVerdict(t, out)
	if v.Decision != "required" {
		t.Fatalf("a Gentle session requires the run: %+v", v)
	}
	if !strings.Contains(v.Reason, "TSP_GENTLE_OBSERVATION") {
		t.Fatalf("the reason must name the signal: %q", v.Reason)
	}
}

// The test a committed unit brought counts as its test: the unit owes no test, only the branch's one TSP run,
// and once a round is on record for the branch the unit owes nothing and the decision says so.
func TestCheckSinceIsSatisfiedByTheTestTheUnitBrought(t *testing.T) {
	t.Setenv("TSP_GENTLE_OBSERVATION", "pi-session-gentle-active")
	t.Setenv("TSP_HOME", t.TempDir())
	// The base already holds the module, so the round that is recorded below can run the unit's test against it.
	dir := pathsRepo(t, "a.go")
	writeIn(t, dir, "go.mod", "module sample\n\ngo 1.22\n")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-qm", "module")
	writeIn(t, dir, "a.go", "package sample\n\nfunc F() int { return 1 }\n")
	writeIn(t, dir, "a_test.go", "package sample\n\nimport \"testing\"\n\nfunc TestF(t *testing.T) {}\n")
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "-qm", "work unit")

	code, out, errOut := runCheckCapturing(t, "--cwd", dir, "--since", "HEAD~1", "--paths", "a.go", "--json")
	v := readVerdict(t, out)
	if code != 1 || v.Decision != "required" || !strings.Contains(v.Debt, "a test came with it (a_test.go)") || !strings.Contains(v.Debt, "no TSP run is on record") {
		t.Fatalf("before any round the unit owes the run, not a test: code=%d stdout=%q stderr=%q", code, out, errOut)
	}

	if rec, rc := findings.Record(dir, "", "", time.Minute); rc == 2 {
		t.Fatalf("record: %s", rec)
	}
	code, out, errOut = runCheckCapturing(t, "--cwd", dir, "--since", "HEAD~1", "--paths", "a.go", "--json")
	if code != 0 {
		t.Fatalf("a unit with its test and a round on record owes nothing: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	if v := readVerdict(t, out); v.Decision != "none" {
		t.Fatalf("nothing to gate: %+v", v)
	}
}

// A repository that declares never still reads the debt and records it; it just does not gate on it.
func TestCheckGateNeverDoesNotGate(t *testing.T) {
	t.Setenv("TSP_GENTLE_OBSERVATION", "pi-session-gentle-active")
	dir := committedUnit(t, map[string]string{
		"a.go":      "package sample\n\nfunc F() int { return 1 }\n",
		".tsp.json": `{"gate":"never"}`,
	})

	code, out, errOut := runCheckCapturing(t, "--cwd", dir, "--since", "HEAD~1", "--paths", "a.go")
	if code != 0 {
		t.Fatalf("a repository that declares never does not gate: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	for _, want := range []string{"no test was added or changed with it", "gate: none", "gate never is declared"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q: %q", want, out)
		}
	}
}

// A --since nobody can resolve is the caller's mistake, never a clean tree.
func TestCheckSinceRefusesARefThatDoesNotResolve(t *testing.T) {
	dir := pathsRepo(t, "a.go")
	code, out, errOut := runCheckCapturing(t, "--cwd", dir, "--since", "nope", "--json")
	if code != 2 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	v := readVerdict(t, out)
	if v.Decision != "error" || !strings.Contains(v.Reason, "nope") {
		t.Fatalf("the refusal must name the ref and carry no level: %+v", v)
	}
}

// The offer shows the debt, never the check's own bookkeeping: the warning about its evidence and the count of
// files that moved say why the check speaks, and only a plan's pending target in parentheses is the debt itself.
func TestDebtLineNamesTheDebtAndNotTheBookkeeping(t *testing.T) {
	const debt = "no test was added or changed with it"
	for _, tc := range []struct{ name, text, want string }{
		{"a warning about the check's own evidence", "warning: git ignores plan/tpp.md\n1 production source file(s) changed: a.go\n" + debt, debt},
		{"the plan data marker", "1 production source file(s) changed: a.go\n(the names below are read from the plan file)\n" + debt, debt},
		{"a pending plan target in parentheses is the debt", "1 production source file(s) changed: m1.go\n(plan m1 still owes 2 targets)", "(plan m1 still owes 2 targets)"},
		{"bookkeeping alone names nothing", "warning: git ignores plan/tpp.md\n1 production source file(s) changed: a.go\n", ""},
		{"an empty answer names nothing", "", ""},
	} {
		if got := check.DebtLine(tc.text); got != tc.want {
			t.Errorf("%s: DebtLine = %q, want %q", tc.name, got, tc.want)
		}
	}
}
