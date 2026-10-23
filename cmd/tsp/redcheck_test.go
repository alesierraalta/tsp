package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// redcheckRepo commits a Go module whose Order breaks its "sorted" contract, then lets the test add files.
func redcheckRepo(t *testing.T, agentTest string) string {
	t.Helper()
	dir := t.TempDir()
	write := func(rel, body string) {
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/rc\n\ngo 1.21\n")
	write("order.go", "package rc\n\nfunc Order() string { return \"cab\" }\n")
	write("order_test.go", "package rc\n\nimport \"testing\"\n\nfunc TestOrderLen(t *testing.T) {\n\tif len(Order()) != 3 {\n\t\tt.Fatal()\n\t}\n}\n")
	for _, args := range [][]string{
		{"init", "-q"}, {"config", "user.email", "t@example.com"}, {"config", "user.name", "t"},
		{"add", "-A"}, {"commit", "-qm", "base"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if agentTest != "" {
		write("agent_test.go", agentTest)
	}
	return dir
}

func runRedcheckCLI(t *testing.T, bin, dir string) (int, string) {
	t.Helper()
	cmd := exec.Command(bin, "redcheck", "--cwd", dir)
	out, err := cmd.CombinedOutput()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), string(out)
	}
	if err != nil {
		t.Fatalf("redcheck: %v", err)
	}
	return 0, string(out)
}

// The command's exit code is the verdict an agent acts on: 0 when at least one new test fails on the unmodified
// code (a defect witness exists), 1 when no new test fails there or none was added, 2 when nothing could be
// judged. A new test that passes beside a witness may check a promise the code keeps, so it is listed but
// never turns the verdict: telling the agent to make it fail would push it to break a correct test.
func TestRedcheckExitCodesAndReport(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the CLI and runs go test")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not installed")
	}
	bin := buildCLI(t)
	const sorted = "func TestOrderIsSorted(t *testing.T) {\n\tif Order() != \"abc\" {\n\t\tt.Fatal(Order())\n\t}\n}\n"
	const blind = "func TestOrderNotEmpty(t *testing.T) {\n\tif Order() == \"\" {\n\t\tt.Fatal()\n\t}\n}\n"
	head := "package rc\n\nimport \"testing\"\n\n"
	cases := []struct {
		name     string
		dir      string
		wantCode int
		want     []string
		absent   []string
	}{
		{"only passing new tests pin nothing", redcheckRepo(t, head+blind), 1,
			[]string{"passes on the unmodified code: example.com/rc.TestOrderNotEmpty", "no new test fails on the unmodified code"}, []string{"fails on the unmodified code:"}},
		{"a passing test beside a witness is listed, not failed", redcheckRepo(t, head+sorted+blind), 0,
			[]string{"fails on the unmodified code: example.com/rc.TestOrderIsSorted", "passes on the unmodified code: example.com/rc.TestOrderNotEmpty"}, []string{"change each"}},
		{"every new test fails", redcheckRepo(t, head+sorted), 0,
			[]string{"fails on the unmodified code: example.com/rc.TestOrderIsSorted", "every new test fails on the unmodified code"}, []string{"pins nothing"}},
		{"no test was added", redcheckRepo(t, ""), 1, []string{"no test file was added or changed"}, nil},
		{"a test that does not compile", redcheckRepo(t, head+"func TestBroken(t *testing.T) { nope() }\n"), 2, []string{"could not be run test by test"}, []string{"fails on the unmodified code"}},
		{"outside a repository", t.TempDir(), 2, []string{"not inside a git repository"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, out := runRedcheckCLI(t, bin, tc.dir)
			if code != tc.wantCode {
				t.Fatalf("exit = %d, want %d\n%s", code, tc.wantCode, out)
			}
			for _, w := range tc.want {
				if !strings.Contains(out, w) {
					t.Errorf("output missing %q:\n%s", w, out)
				}
			}
			for _, a := range tc.absent {
				if strings.Contains(out, a) {
					t.Errorf("output must not contain %q:\n%s", a, out)
				}
			}
		})
	}
}

// A test file pytest cannot import gives no verdict: the witnesses that did collect still judge the change,
// and the file that did not is named with its reason instead of hiding behind a failed run.
func TestRedcheckNamesAFilePytestCouldNotCollect(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the CLI and runs pytest")
	}
	if err := exec.Command("python3", "-m", "pytest", "--version").Run(); err != nil {
		t.Skip("pytest not installed")
	}
	bin := buildCLI(t)
	dir := t.TempDir()
	write := func(rel, body string) {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, rel)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("calc.py", "def add(a, b):\n    return a - b\n")
	for _, args := range [][]string{
		{"init", "-q"}, {"config", "user.email", "t@example.com"}, {"config", "user.name", "t"},
		{"add", "-A"}, {"commit", "-qm", "base"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	write("tests/test_calc.py", "from calc import add\n\ndef test_add():\n    assert add(2, 2) == 4\n")
	write("tests/test_db.py", "import tsp_missing_module\n\ndef test_db():\n    pass\n")
	code, out := runRedcheckCLI(t, bin, dir)
	for _, want := range []string{
		"fails on the unmodified code: tests/test_calc.py::test_add",
		"could not collect: tests/test_db.py (ModuleNotFoundError: No module named 'tsp_missing_module')",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if code != 0 {
		t.Errorf("exit %d, want 0 (a witness exists):\n%s", code, out)
	}
}
