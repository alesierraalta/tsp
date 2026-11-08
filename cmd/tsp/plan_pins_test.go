package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// pinRepo writes a Go module whose one package holds a test that fails while the defect is present, and the same
// package with a test that passes, so a row can claim either.
func pinRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"go.mod":       "module m\n\ngo 1.21\n",
		"calc/calc.go": "package calc\n\nfunc Add(a, b int) int { return a - b }\n",
		"calc/red_test.go": `package calc

import "testing"

func TestAddFails(t *testing.T) {
	if Add(2, 1) != 3 {
		t.Fatalf("Add(2, 1) = %d, want 3", Add(2, 1))
	}
}
`,
		"calc/green_test.go": `package calc

import "testing"

func TestAddPassesForZero(t *testing.T) {
	if Add(0, 0) != 0 {
		t.Fatal("Add(0, 0) must be 0")
	}
}
`,
	}
	for name, body := range files {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}, {"-c", "user.email=t@t", "-c", "user.name=t", "commit", "-qm", "base"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	if out, code := runCLI(t, buildCLI(t), "plan", "init", "--micro", "--path", filepath.Join(dir, "plan.md")); code != 0 {
		t.Fatalf("plan init = %d %s", code, out)
	}
	return dir
}

func addRow(t *testing.T, bin, dir, test string) {
	t.Helper()
	plan := filepath.Join(dir, "plan.md")
	if out, code := runCLI(t, bin, "plan", "add-evidence", "--path", plan, "--claim", "c", "--executed", "e", "--admit", "go test ./calc", "--inputs", "i", "--observed", "o", "--label", "observado"); code != 0 {
		t.Fatalf("add-evidence = %d %s", code, out)
	}
	if out, code := runCLI(t, bin, "plan", "add-finding", "--path", plan, "--location", "calc/calc.go:3", "--severity", "silently wrong answer",
		"--data-safe", "yes", "--evidence", "E1", "--status", "confirmed", "--test", test, "--verdict-by", "t / 2026-11-08", "--reason", "r"); code != 0 {
		t.Fatalf("add-finding = %d %s", code, out)
	}
}

// A row that names a test as its witness claims that test fails while the defect is there. --pins runs each such
// test and refuses the row whose test passes, which is a check that checks nothing.
func TestPlanCheckPinsRefusesATestThatPasses(t *testing.T) {
	bin := buildCLI(t)
	dir := pinRepo(t)
	plan := filepath.Join(dir, "plan.md")
	addRow(t, bin, dir, "calc/red_test.go :: TestAddFails")

	out, code := runCLI(t, bin, "plan", "check", "--path", plan, "--pins")
	if code != 0 || !strings.Contains(out, "well formed") {
		t.Fatalf("a failing witness must pass: %d %s", code, out)
	}

	dir2 := pinRepo(t)
	plan2 := filepath.Join(dir2, "plan.md")
	addRow(t, bin, dir2, "calc/green_test.go :: TestAddPassesForZero")
	out, code = runCLI(t, bin, "plan", "check", "--path", plan2, "--pins")
	if code != 1 || !strings.Contains(out, "passes on this code, so it pins nothing") {
		t.Fatalf("a passing witness must be refused: %d %s", code, out)
	}
	if plain, code := runCLI(t, bin, "plan", "check", "--path", plan2); code != 0 || !strings.Contains(plain, "well formed") {
		t.Fatalf("without --pins nothing changes: %d %s", code, plain)
	}
}

// A witness naming a file the repository does not have cannot be run, so the check says so instead of passing.
func TestPlanCheckPinsNamesAWitnessItCannotRun(t *testing.T) {
	bin := buildCLI(t)
	dir := pinRepo(t)
	plan := filepath.Join(dir, "plan.md")
	addRow(t, bin, dir, "calc/missing_test.go :: TestGone")
	out, code := runCLI(t, bin, "plan", "check", "--path", plan, "--pins")
	if code != 1 || !strings.Contains(out, "cannot be run") {
		t.Fatalf("missing witness = %d %s", code, out)
	}
}

// The hosts ask the CLI how much of the plan a run recorded, so the answer is machine-readable and counts what
// the plan holds (#398).
func TestPlanCountReportsTheRowsARunRecorded(t *testing.T) {
	bin := buildCLI(t)
	dir := pinRepo(t)
	plan := filepath.Join(dir, "plan.md")
	out, code := runCLI(t, bin, "plan", "count", "--path", plan, "--json")
	if code != 0 || strings.TrimSpace(out) != `{"evidence":0,"findings":0}` {
		t.Fatalf("empty plan count = %d %q", code, out)
	}
	addRow(t, bin, dir, "calc/red_test.go :: TestAddFails")
	out, code = runCLI(t, bin, "plan", "count", "--path", plan, "--json")
	if code != 0 || strings.TrimSpace(out) != `{"evidence":1,"findings":1}` {
		t.Fatalf("counted plan = %d %q", code, out)
	}
	if human, code := runCLI(t, bin, "plan", "count", "--path", plan); code != 0 || !strings.Contains(human, "1 finding(s), 1 evidence row(s)") {
		t.Fatalf("human count = %d %q", code, human)
	}
}

// A row that says its defect came with the change must cite a file the change touches: a defect on an untouched
// file was already there, and claiming otherwise blocks a merge for something this change did not do (#402).
func TestPlanCheckBaseRefusesAnIntroducedFindingOnAnUntouchedFile(t *testing.T) {
	bin := buildCLI(t)
	dir := pinRepo(t)
	if out, err := exec.Command("git", "-C", dir, "branch", "-m", "main").CombinedOutput(); err != nil {
		t.Fatalf("branch main: %v %s", err, out)
	}
	if out, err := exec.Command("git", "-C", dir, "checkout", "-q", "-b", "change").CombinedOutput(); err != nil {
		t.Fatalf("branch: %v %s", err, out)
	}
	if err := os.WriteFile(filepath.Join(dir, "calc/calc.go"), []byte("package calc\n\nfunc Add(a, b int) int { return a + b }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", dir, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-qam", "fix").CombinedOutput(); err != nil {
		t.Fatalf("commit: %v %s", err, out)
	}
	plan := filepath.Join(dir, "plan.md")
	if out, code := runCLI(t, bin, "plan", "add-evidence", "--path", plan, "--claim", "c", "--executed", "e", "--admit", "go test ./calc", "--inputs", "i", "--observed", "o", "--label", "observado"); code != 0 {
		t.Fatalf("add-evidence = %d %s", code, out)
	}
	add := func(location, causality string) {
		if out, code := runCLI(t, bin, "plan", "add-finding", "--path", plan, "--location", location, "--severity", "silently wrong answer",
			"--data-safe", "yes", "--evidence", "E1", "--status", "open", "--verdict-by", "t / 2026-11-08", "--reason", "r", "--causality", causality); code != 0 {
			t.Fatalf("add-finding %s = %d %s", location, code, out)
		}
	}
	add("calc/calc.go:3", "introduced")
	if out, code := runCLI(t, bin, "plan", "check", "--path", plan, "--base", "main"); code != 0 {
		t.Fatalf("a finding on a touched file passes: %d %s", code, out)
	}
	add("calc/other.go:9", "introduced")
	out, code := runCLI(t, bin, "plan", "check", "--path", plan, "--base", "main")
	if code != 1 || !strings.Contains(out, "is not touched by the change against main") {
		t.Fatalf("an untouched file must be refused: %d %s", code, out)
	}
}
