package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// pinRepo builds a module with one test that fails and one that passes, a plan started from the real template, and
// a review artifact naming the defect, so the whole contract can be exercised end to end.
func pinWorkdir(t *testing.T) (dir, plan, findings string) {
	t.Helper()
	bin := buildCLI(t)
	dir = t.TempDir()
	for name, body := range map[string]string{
		"go.mod":         "module m\n\ngo 1.21\n",
		"calc/c.go":      "package calc\n\nfunc Add(a, b int) int { return a - b }\n",
		"calc/r_test.go": "package calc\n\nimport \"testing\"\n\nfunc TestAddFails(t *testing.T) {\n\tif Add(2, 1) != 3 {\n\t\tt.Fatalf(\"Add(2, 1) = %d\", Add(2, 1))\n\t}\n}\n",
		"calc/g_test.go": "package calc\n\nimport \"testing\"\n\nfunc TestAddZero(t *testing.T) {\n\tif Add(0, 0) != 0 {\n\t\tt.Fatal(\"zero\")\n\t}\n}\n",
	} {
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
	plan = filepath.Join(dir, "plan.md")
	if out, code := runCLI(t, bin, "plan", "init", "--micro", "--path", plan); code != 0 {
		t.Fatalf("plan init = %d %s", code, out)
	}
	findings = filepath.Join(dir, "review.json")
	artifact := `{"findings":[{"location":"calc/c.go:3","claim":"Add subtracts instead of adding","severity":"BLOCKER"}]}`
	if err := os.WriteFile(findings, []byte(artifact), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, plan, findings
}

func pinCLI(t *testing.T, dir string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(buildCLI(t), args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err == nil {
		return string(out), 0
	}
	var ee exec.ExitError
	if asExit(err, &ee) {
		return string(out), ee.ExitCode()
	}
	t.Fatalf("pin %v: %v", args, err)
	return "", -1
}

// A pin is only a pin when the test fails on this code: recording a passing test is refused and nothing changes.
func TestPinRecordsOnlyATestThatFails(t *testing.T) {
	dir, planRef, findings := pinWorkdir(t)
	bin := buildCLI(t)
	if out, code := runCLI(t, bin, "pin", "--path", planRef, "--findings", findings); code != 0 {
		t.Fatalf("import = %d %s", code, out)
	}
	before, _ := os.ReadFile(planRef)
	if out, code := pinCLI(t, dir, "pin", "--path", planRef, "--record", "P1", "--test", "calc/g_test.go :: TestAddZero"); code != 1 || !strings.Contains(out, "pins nothing") {
		t.Fatalf("a passing witness must be refused: %d %s", code, out)
	}
	after, _ := os.ReadFile(planRef)
	if string(before) != string(after) {
		t.Fatal("a refused record must leave the plan untouched")
	}
	if out, code := pinCLI(t, dir, "pin", "--path", planRef, "--record", "P1", "--test", "calc/r_test.go :: TestAddFails"); code != 0 {
		t.Fatalf("a failing witness must be recorded: %d %s", code, out)
	}
	body, _ := os.ReadFile(planRef)
	if !strings.Contains(string(body), "| P1 | calc/c.go:3 | Add subtracts instead of adding | pinned | calc/r_test.go :: TestAddFails | - |") {
		t.Fatalf("the pin was not recorded: %s", body)
	}
	if out, code := runCLI(t, bin, "plan", "check", "--path", planRef); code != 0 {
		t.Fatalf("the plan must stay valid: %d %s", code, out)
	}
}

// Blocked and rejected carry the reason that says why nothing was pinned, and the summary counts what is where.
func TestPinBlockedRejectedAndTheSummary(t *testing.T) {
	dir, planRef, findings := pinWorkdir(t)
	bin := buildCLI(t)
	if out, code := runCLI(t, bin, "pin", "--path", planRef, "--findings", findings); code != 0 {
		t.Fatalf("import = %d %s", code, out)
	}
	if out, code := pinCLI(t, dir, "pin", "--path", planRef, "--block", "P1", "--reason", "needs a fixture this repository does not have"); code != 0 {
		t.Fatalf("block = %d %s", code, out)
	}
	if out, code := pinCLI(t, dir, "pin", "--path", planRef, "--block", "P1"); code != 2 || !strings.Contains(out, "--reason") {
		t.Fatalf("block without a reason must be refused: %d %s", code, out)
	}
	out, code := pinCLI(t, dir, "pin", "--path", planRef)
	if code != 0 || !strings.Contains(out, "0 pinned, 0 open, 1 blocked, 0 rejected") {
		t.Fatalf("summary = %d %s", code, out)
	}
	if out, code := pinCLI(t, dir, "pin", "--path", planRef, "--refute", "P1", "--reason", "the claim does not reproduce"); code != 0 {
		t.Fatalf("refute = %d %s", code, out)
	}
	if out, code := runCLI(t, bin, "plan", "check", "--path", planRef); code != 0 {
		t.Fatalf("the plan must stay valid: %d %s", code, out)
	}
	if out, code := pinCLI(t, dir, "pin", "--path", planRef, "--record", "P9", "--test", "calc/r_test.go :: TestAddFails"); code != 1 || !strings.Contains(out, "no pinning row P9") {
		t.Fatalf("an unknown pin must be refused: %d %s", code, out)
	}
}

// Importing the same review twice does not double the list, and a finding with no path:line is refused.
func TestPinImportIsIdempotentAndNeedsALocation(t *testing.T) {
	dir, planRef, findings := pinWorkdir(t)
	bin := buildCLI(t)
	for i := 0; i < 2; i++ {
		if out, code := runCLI(t, bin, "pin", "--path", planRef, "--findings", findings); code != 0 {
			t.Fatalf("import %d = %d %s", i, code, out)
		}
	}
	body, _ := os.ReadFile(planRef)
	if strings.Count(string(body), "| P1 |") != 1 {
		t.Fatalf("a repeated import doubled the row: %s", body)
	}
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte(`{"findings":[{"claim":"no location"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, code := runCLI(t, bin, "pin", "--path", planRef, "--findings", bad); code != 1 || !strings.Contains(out, "cites no path:line") {
		t.Fatalf("a finding with no location must be refused: %d %s", code, out)
	}
}

// A plan whose Pinning section breaks its own rules is refused by plan check: a pinned row with no test, a bad
// status, a reasonless rejection.
func TestPlanCheckHoldsThePinningSectionToItsRules(t *testing.T) {
	_, planRef, findings := pinWorkdir(t)
	bin := buildCLI(t)
	if out, code := runCLI(t, bin, "pin", "--path", planRef, "--findings", findings); code != 0 {
		t.Fatalf("import = %d %s", code, out)
	}
	body, _ := os.ReadFile(planRef)
	for name, broken := range map[string][3]string{
		"no test":    {"| P1 | calc/c.go:3 | c | pinned | - | - |", "names no test", ""},
		"bad status": {"| P1 | calc/c.go:3 | c | maybe | x_test.go :: T | - |", "status must be", ""},
		"no reason":  {"| P1 | calc/c.go:3 | c | rejected | - | - |", "gives no reason", ""},
	} {
		edited := strings.Replace(string(body), "| P1 | calc/c.go:3 | Add subtracts instead of adding | open | - | - |", broken[0], 1)
		if err := os.WriteFile(planRef, []byte(edited), 0o644); err != nil {
			t.Fatal(err)
		}
		if out, code := runCLI(t, bin, "plan", "check", "--path", planRef); code != 1 || !strings.Contains(out, broken[1]) {
			t.Fatalf("%s: %d %s", name, code, out)
		}
	}
}

// A row names its witness from the repository root, which is where every plan in this repository lives:
// docs/testing/test-plan.md is two directories below it. Resolving the witness from the plan's own directory
// looks for calc/c_test.go under docs/testing and reports a test that is there as "no such file", so a finding
// the run actually pinned is recorded as blocked (#430).
func TestPinRecordsAWitnessForAPlanOutsideTheRepositoryRoot(t *testing.T) {
	dir, _, findings := pinWorkdir(t)
	bin := buildCLI(t)
	plan := filepath.Join(dir, "docs", "testing", "test-plan.md")
	if err := os.MkdirAll(filepath.Dir(plan), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, code := runCLI(t, bin, "plan", "init", "--micro", "--path", plan); code != 0 {
		t.Fatalf("plan init = %d %s", code, out)
	}
	if out, code := runCLI(t, bin, "pin", "--path", plan, "--findings", findings); code != 0 {
		t.Fatalf("import = %d %s", code, out)
	}
	out, code := pinCLI(t, dir, "pin", "--path", plan, "--record", "P1", "--test", "calc/r_test.go :: TestAddFails")
	if code != 0 {
		t.Fatalf("record = %d %s: a witness that is in the repository was called missing", code, out)
	}
	body, err := os.ReadFile(plan)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "| pinned |") {
		t.Fatalf("plan = %s, want the row recorded as pinned", body)
	}
}
