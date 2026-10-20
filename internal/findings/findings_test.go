package findings_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alesierraalta/tsp/internal/findings"
)

const wait = 30 * time.Second
const redTest = "package sample\nimport \"testing\"\n// Keeps the promise.\nfunc TestPromise(t *testing.T) { if Value() != 1 { t.Fatal(\"broken\") } }\n"

func command(t *testing.T, dir string, args ...string) string {
	t.Helper()
	c := exec.Command(args[0], args[1:]...)
	c.Dir = dir
	b, e := c.CombinedOutput()
	if e != nil {
		t.Fatalf("%v: %v: %s", args, e, b)
	}
	return strings.TrimSpace(string(b))
}
func put(t *testing.T, dir, name, body string) {
	t.Helper()
	if e := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); e != nil {
		t.Fatal(e)
	}
}
func repo(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("real git and Go runners")
	}
	t.Setenv("TSP_HOME", t.TempDir())
	d := t.TempDir()
	command(t, d, "git", "init", "-b", "topic")
	command(t, d, "git", "config", "user.email", "test@example.invalid")
	command(t, d, "git", "config", "user.name", "Test")
	put(t, d, "go.mod", "module example.invalid/sample\n\ngo 1.22\n")
	put(t, d, "value.go", "package sample\nfunc Value() int { return 0 }\n")
	command(t, d, "git", "add", ".")
	command(t, d, "git", "commit", "-qm", "base")
	return d
}
func record(t *testing.T, d string) {
	t.Helper()
	out, code := findings.Record(d, "", wait)
	want := "F1 recorded value_test.go example.invalid/sample.TestPromise\n"
	if code != 0 || out != want {
		t.Fatalf("record exit=%d: %q want %q", code, out, want)
	}
}
func review(t *testing.T, d string, n int, status string, fixed, open, newCount, regressed, need, code int) {
	t.Helper()
	before := command(t, d, "git", "status", "--porcelain", "--untracked-files=all")
	out, got := findings.Check(d, wait)
	want := fmt.Sprintf("review %d on topic: %d fixed, %d open, %d new, %d regressed", n, fixed, open, newCount, regressed)
	if need > 0 {
		want += fmt.Sprintf(", %d need review", need)
	}
	want += "\nF1 " + status + " value_test.go example.invalid/sample.TestPromise — Keeps the promise.\n"
	if got != code || out != want {
		t.Fatalf("check exit=%d want=%d\ngot: %q\nwant: %q", got, code, out, want)
	}
	if after := command(t, d, "git", "status", "--porcelain", "--untracked-files=all"); after != before {
		t.Fatalf("repository changed: %q -> %q", before, after)
	}
}
func TestD1StorageAndBranchIsolation(t *testing.T) {
	d := repo(t)
	put(t, d, "value_test.go", redTest)
	record(t, d)
	files, e := filepath.Glob(filepath.Join(os.Getenv("TSP_HOME"), "findings", "*", "topic.json"))
	if e != nil || len(files) != 1 {
		t.Fatalf("registry under TSP_HOME: %v %v", files, e)
	}
	command(t, d, "git", "switch", "-c", "other/branch")
	out, c := findings.Check(d, wait)
	if c != 0 || out != "no findings recorded on other/branch\n" {
		t.Fatalf("separate branch: %d %q", c, out)
	}
	record(t, d)
	files, _ = filepath.Glob(filepath.Join(os.Getenv("TSP_HOME"), "findings", "*", "*.json"))
	if len(files) != 2 {
		t.Fatalf("branch registries: %v", files)
	}
	command(t, d, "git", "switch", "--detach")
	if out, c := findings.Check(d, wait); c != 2 {
		t.Fatalf("detached exit=%d: %s", c, out)
	}
	if out, c := findings.Record(d, "", wait); c != 2 {
		t.Fatalf("detached record exit=%d: %s", c, out)
	}
}
func TestD2RecordRefreshesWitness(t *testing.T) {
	d := repo(t)
	put(t, d, "value_test.go", redTest)
	record(t, d)
	files, _ := filepath.Glob(filepath.Join(os.Getenv("TSP_HOME"), "findings", "*", "*.json"))
	read := func() findings.Finding {
		t.Helper()
		data, err := os.ReadFile(files[0])
		if err != nil {
			t.Fatal(err)
		}
		var stored struct{ Findings []findings.Finding }
		if err := json.Unmarshal(data, &stored); err != nil || len(stored.Findings) != 1 {
			t.Fatalf("stored witnesses: %s %v", data, err)
		}
		return stored.Findings[0]
	}
	original := read()
	if original.ID != "F1" || original.File != "value_test.go" || original.Test != "example.invalid/sample.TestPromise" || original.Suite != "go test -count=1 ." || original.Promise != "Keeps the promise." || len(original.Fingerprint) != 64 || original.Time.IsZero() || original.Commit != command(t, d, "git", "rev-parse", "HEAD") {
		t.Fatalf("witness metadata: %+v", original)
	}
	record(t, d)
	review(t, d, 1, "open", 0, 0, 1, 0, 0, 1)
	put(t, d, "value_test.go", strings.ReplaceAll(redTest, "broken", "still broken"))
	record(t, d)
	refreshed := read()
	if refreshed.ID != original.ID || refreshed.Fingerprint == original.Fingerprint || !refreshed.Time.After(original.Time) {
		t.Fatalf("refresh did not update witness: before=%+v after=%+v", original, refreshed)
	}
	review(t, d, 2, "open", 0, 1, 0, 0, 0, 1)
	put(t, d, "value_test.go", redTest+"func TestSecond(t *testing.T) { t.Fatal(\"second defect\") }\n")
	out, code := findings.Record(d, "", wait)
	want := "F1 recorded value_test.go example.invalid/sample.TestPromise\nF2 recorded value_test.go example.invalid/sample.TestSecond\n"
	if code != 0 || out != want {
		t.Fatalf("next id: %d %q want %q", code, out, want)
	}
	out, code = findings.Check(d, wait)
	want = "review 3 on topic: 0 fixed, 1 open, 1 new, 0 regressed\nF1 open value_test.go example.invalid/sample.TestPromise — Keeps the promise.\nF2 open value_test.go example.invalid/sample.TestSecond\n"
	if code != 1 || out != want {
		t.Fatalf("multiple witnesses: %d %q want %q", code, out, want)
	}
}
func TestD3ChangedBodyCannotBeFixed(t *testing.T) {
	d := repo(t)
	put(t, d, "value_test.go", redTest)
	record(t, d)
	put(t, d, "value_test.go", strings.ReplaceAll(redTest, "!= 1", "!= 0"))
	review(t, d, 1, "test-changed", 0, 0, 0, 0, 1, 1)
}
func TestPythonDecoratorChangeNeedsReview(t *testing.T) {
	d := repo(t)
	if out, err := exec.Command("python3", "-m", "pytest", "--version").CombinedOutput(); err != nil {
		t.Skipf("pytest unavailable: %s", out)
	}
	put(t, d, "value.py", "value = 1\n")
	command(t, d, "git", "add", "value.py")
	command(t, d, "git", "commit", "-qm", "python defect")
	source := "import pytest\nfrom value import value\n# Keeps the parameter promise.\n@pytest.mark.parametrize(\n    \"want\", [pytest.param(2, id=\"\"\"expected\"\"\", marks=pytest.mark.skipif(False, reason='''brackets ) ]\n    multiline string'''))],\n)\ndef test_value(want):\n    assert value == want\n"
	put(t, d, "test_value.py", source)
	if out, code := findings.Record(d, "python3 -m pytest test_value.py", wait); code != 0 {
		t.Fatalf("record: %d %s", code, out)
	}
	put(t, d, "value.py", "value = 2\n")
	out, code := findings.Check(d, wait)
	want := "review 1 on topic: 1 fixed, 0 open, 0 new, 0 regressed\nF1 fixed test_value.py test_value.py::test_value[expected] — Keeps the parameter promise.\n"
	if code != 0 || out != want {
		t.Fatalf("unchanged decorated test: exit=%d got=%q want=%q", code, out, want)
	}
	put(t, d, "value.py", "value = 1\n")
	put(t, d, "test_value.py", strings.ReplaceAll(source, "pytest.param(2,", "pytest.param(1,"))
	out, code = findings.Check(d, wait)
	want = "review 2 on topic: 0 fixed, 0 open, 0 new, 0 regressed, 1 need review\nF1 test-changed test_value.py test_value.py::test_value[expected] — Keeps the parameter promise.\n"
	if code != 1 || out != want {
		t.Fatalf("decorator change: exit=%d got=%q want=%q", code, out, want)
	}
}

func TestD4StatusTransitions(t *testing.T) {
	d := repo(t)
	put(t, d, "value_test.go", redTest)
	record(t, d)
	review(t, d, 1, "open", 0, 0, 1, 0, 0, 1)
	put(t, d, "value.go", "package sample\nfunc Value() int { return 1 }\n")
	command(t, d, "git", "add", ".")
	command(t, d, "git", "commit", "-qm", "fix")
	review(t, d, 2, "fixed", 1, 0, 0, 0, 0, 0)
	review(t, d, 3, "fixed", 0, 0, 0, 0, 0, 0)
	put(t, d, "value.go", "package sample\nfunc Value() int { return 0 }\n")
	review(t, d, 4, "regressed", 0, 0, 0, 1, 0, 1)
	put(t, d, "value.go", "not valid Go")
	review(t, d, 5, "unknown", 0, 0, 0, 0, 1, 2)
	put(t, d, "value.go", "package sample\nfunc Value() int { return 0 }\n")
	put(t, d, "value_test.go", "package sample\n")
	review(t, d, 6, "missing", 0, 0, 0, 0, 1, 1)
}
func TestD5ReportWithoutPromise(t *testing.T) {
	d := repo(t)
	put(t, d, "value_test.go", strings.ReplaceAll(redTest, "// Keeps the promise.\n", ""))
	record(t, d)
	out, c := findings.Check(d, wait)
	want := "review 1 on topic: 0 fixed, 0 open, 1 new, 0 regressed\nF1 open value_test.go example.invalid/sample.TestPromise\n"
	if c != 1 || out != want {
		t.Fatalf("report: %d %q want %q", c, out, want)
	}
}
func TestD6ExitCodes(t *testing.T) {
	d := repo(t)
	if out, c := findings.Record(d, "", wait); c != 1 {
		t.Fatalf("no witness: %d %s", c, out)
	}
	if out, c := findings.Check(d, wait); c != 0 || out != "no findings recorded on topic\n" {
		t.Fatalf("empty: %d %q", c, out)
	}
	if out, c := findings.Check(t.TempDir(), wait); c != 2 {
		t.Fatalf("not git: %d %s", c, out)
	}
	put(t, d, "value_test.go", redTest)
	record(t, d)
	files, _ := filepath.Glob(filepath.Join(os.Getenv("TSP_HOME"), "findings", "*", "*.json"))
	if len(files) != 1 {
		t.Fatal(files)
	}
	if e := os.WriteFile(files[0], []byte("{broken"), 0600); e != nil {
		t.Fatal(e)
	}
	if out, c := findings.Check(d, wait); c != 2 {
		t.Fatalf("corrupt: %d %s", c, out)
	}
}
func TestD7UnsupportedRunnerRefused(t *testing.T) {
	d := repo(t)
	put(t, d, "value_test.go", redTest)
	out, c := findings.Record(d, "cargo test", wait)
	if c != 2 || !strings.Contains(out, "go test, pytest, node --test") {
		t.Fatalf("unsupported: %d %q", c, out)
	}
	entries, _ := os.ReadDir(os.Getenv("TSP_HOME"))
	if len(entries) != 0 {
		t.Fatalf("unsupported runner wrote registry: %v", entries)
	}
}
