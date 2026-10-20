package bench

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestCargoQuietInlineDiagnostic(t *testing.T) {
	if testing.Short() {
		t.Skip("real cargo runner")
	}
	if _, err := exec.LookPath("cargo"); err != nil {
		t.Skip("cargo unavailable")
	}
	dir := redCheckRepo(t)
	writeFile(t, dir, "Cargo.toml", "[package]\nname = \"sample\"\nversion = \"0.1.0\"\nedition = \"2021\"\n[lib]\npath = \"lib.rs\"\n")
	writeFile(t, dir, "lib.rs", "#[test]\nfn promise() { assert_eq!(1, 2); }\n")
	_, err := RedCheck(dir, "cargo test --quiet", time.Minute)
	if err == nil || err.Error() != "promise: inline Rust unit tests are not supported; move the test to tests/" {
		t.Fatalf("diagnostic: %v", err)
	}
}

func TestInferPackageTestRunner(t *testing.T) {
	for _, tc := range []struct{ script, want string }{
		{"jest --runInBand", "npx jest a.test.ts"},
		{"npx vitest run --coverage", "npx vitest run a.test.ts"},
		{"vitest", "npx vitest run a.test.ts"},
		{"node scripts/jest-helper.js", "node --test a.test.ts"},
		{"echo jest", "node --test a.test.ts"},
		{"node --test", "node --test a.test.ts"},
	} {
		t.Run(tc.script, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "package.json", `{"scripts":{"test":"`+tc.script+`"}}`)
			got, err := inferRedCheckSuite(dir, []string{"a.test.ts"})
			if err != nil || got != tc.want {
				t.Fatalf("got %q %v, want %q", got, err, tc.want)
			}
		})
	}
}

// redCheckRepo is a git repository whose HEAD holds a Go module with one defect: Order returns "cab"
// where its contract promises "abc". It also carries an existing table test the agent can extend.
func redCheckRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"go.mod":        "module example.com/rc\n\ngo 1.21\n",
		"order.go":      "package rc\n\nfunc Order() string { return \"cab\" }\n\nfunc Len() int { return len(Order()) }\n",
		"order_test.go": "package rc\n\nimport \"testing\"\n\nfunc TestLen(t *testing.T) {\n\tfor _, c := range []struct{ name string; want int }{{\"three\", 3}} {\n\t\tt.Run(c.name, func(t *testing.T) {\n\t\t\tif Len() != c.want {\n\t\t\t\tt.Fatal(Len())\n\t\t\t}\n\t\t})\n\t}\n}\n",
	}
	for rel, body := range files {
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, args := range [][]string{
		{"init", "-q"}, {"config", "user.email", "t@example.com"}, {"config", "user.name", "t"},
		{"add", "-A"}, {"commit", "-qm", "base"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func writeFile(t *testing.T, dir, rel, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, rel)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, rel), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A new test that passes on the unmodified code pins nothing; one that fails there is a witness. Only tests
// the agent added are judged: the suite's existing tests pass by construction and say nothing. A new row in
// an existing table test is a new test too, judged by its own name.
func TestRedCheckNamesNewTestsThatPassOnTheUnmodifiedCode(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go test")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not installed")
	}
	dir := redCheckRepo(t)
	writeFile(t, dir, "agent_test.go", "package rc\n\nimport \"testing\"\n\n"+
		"func TestOrderIsSorted(t *testing.T) {\n\tif Order() != \"abc\" {\n\t\tt.Fatal(Order())\n\t}\n}\n\n"+
		"func TestOrderHasThreeLetters(t *testing.T) {\n\tif len(Order()) != 3 {\n\t\tt.Fatal(Order())\n\t}\n}\n")
	writeFile(t, dir, "order_test.go", "package rc\n\nimport \"testing\"\n\nfunc TestLen(t *testing.T) {\n\tfor _, c := range []struct{ name string; want int }{{\"three\", 3}, {\"still three\", 3}} {\n\t\tt.Run(c.name, func(t *testing.T) {\n\t\t\tif Len() != c.want {\n\t\t\t\tt.Fatal(Len())\n\t\t\t}\n\t\t})\n\t}\n}\n")
	// The agent's own edit to the source must not decide the verdict: the check runs on HEAD's code.
	writeFile(t, dir, "order.go", "package rc\n\nfunc Order() string { return \"abc\" }\n\nfunc Len() int { return len(Order()) }\n")

	got, err := RedCheck(dir, "", 2*time.Minute)
	if err != nil {
		t.Fatalf("RedCheck: %v", err)
	}
	if want := []string{"agent_test.go", "order_test.go"}; !reflect.DeepEqual(got.TestFiles, want) {
		t.Errorf("test files = %v, want %v", got.TestFiles, want)
	}
	if want := []string{"example.com/rc.TestOrderIsSorted"}; !reflect.DeepEqual(got.Red, want) {
		t.Errorf("red = %v, want %v", got.Red, want)
	}
	if want := []string{"example.com/rc.TestLen/still_three", "example.com/rc.TestOrderHasThreeLetters"}; !reflect.DeepEqual(got.PinsNothing, want) {
		t.Errorf("pins nothing = %v, want %v", got.PinsNothing, want)
	}
}

// A row added to an existing table test that runs its cases without subtests keeps the test's name, so the
// name alone cannot say it is new. When the agent's version of a test that passed at HEAD fails on HEAD's code,
// the agent's change made it red: it is a witness. A test that already failed at HEAD says nothing.
func TestRedCheckCountsAnExistingTestTheAgentTurnedRed(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go test")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not installed")
	}
	dir := redCheckRepo(t)
	writeFile(t, dir, "order_test.go", "package rc\n\nimport \"testing\"\n\nfunc TestLen(t *testing.T) {\n\tfor _, want := range []int{3} {\n\t\tif Len() != want {\n\t\t\tt.Fatal(Len())\n\t\t}\n\t}\n}\n\nfunc TestOrderTable(t *testing.T) {\n\tfor _, want := range []string{\"abc\"} {\n\t\tif Order() != want {\n\t\t\tt.Fatal(Order())\n\t\t}\n\t}\n}\n")
	if out, err := exec.Command("git", "-C", dir, "commit", "-qam", "TestOrderTable already red at HEAD").CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}
	// The agent adds a row to TestLen (passing at HEAD) that the defect breaks, and a row to TestOrderTable,
	// which was red before the agent touched it.
	writeFile(t, dir, "order_test.go", "package rc\n\nimport \"testing\"\n\nfunc TestLen(t *testing.T) {\n\tfor _, want := range []int{3, 4} {\n\t\tif Len() != want {\n\t\t\tt.Fatal(Len())\n\t\t}\n\t}\n}\n\nfunc TestOrderTable(t *testing.T) {\n\tfor _, want := range []string{\"abc\", \"cab\"} {\n\t\tif Order() != want {\n\t\t\tt.Fatal(Order())\n\t\t}\n\t}\n}\n")
	got, err := RedCheck(dir, "", 2*time.Minute)
	if err != nil {
		t.Fatalf("RedCheck: %v", err)
	}
	if want := []string{"example.com/rc.TestLen"}; !reflect.DeepEqual(got.Red, want) {
		t.Errorf("red = %v, want %v", got.Red, want)
	}
	if len(got.PinsNothing) != 0 {
		t.Errorf("pins nothing = %v, want none: an existing test that keeps its verdict is not judged", got.PinsNothing)
	}
}

// With no test file added or changed there is nothing to judge: the report says so instead of passing.
func TestRedCheckReportsNoNewTests(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go test")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not installed")
	}
	got, err := RedCheck(redCheckRepo(t), "", 2*time.Minute)
	if err != nil {
		t.Fatalf("RedCheck: %v", err)
	}
	if len(got.TestFiles) != 0 || len(got.Red) != 0 || len(got.PinsNothing) != 0 {
		t.Fatalf("report = %+v, want empty", got)
	}
}

// A test that does not compile proves nothing: a failed build must never read as a red witness.
func TestRedCheckDoesNotCountABuildFailureAsRed(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go test")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not installed")
	}
	dir := redCheckRepo(t)
	writeFile(t, dir, "agent_test.go", "package rc\n\nimport \"testing\"\n\nfunc TestBroken(t *testing.T) { undefinedHelper() }\n")
	got, err := RedCheck(dir, "", 2*time.Minute)
	if err == nil {
		t.Fatalf("RedCheck on a test that does not compile returned no error: %+v", got)
	}
	if len(got.Red) != 0 {
		t.Fatalf("a build failure was reported as red: %v", got.Red)
	}
}

// Outside a git repository there is no unmodified code to compare against.
func TestRedCheckRefusesADirectoryOutsideARepository(t *testing.T) {
	if _, err := RedCheck(t.TempDir(), "", time.Minute); err == nil {
		t.Fatal("RedCheck outside a repository returned no error")
	}
}

// pytest is read test by test like go test and node --test: a new test that fails on HEAD's code is a
// witness, one that passes there pins nothing, and the suite is inferred from the changed .py tests.
func TestRedCheckReadsPytestTestByTest(t *testing.T) {
	if testing.Short() {
		t.Skip("runs pytest")
	}
	if err := exec.Command("python3", "-m", "pytest", "--version").Run(); err != nil {
		t.Skip("pytest not installed")
	}
	dir := t.TempDir()
	writeFile(t, dir, "app/__init__.py", "")
	writeFile(t, dir, "app/order.py", "def order():\n    return 'cab'\n")
	writeFile(t, dir, "tests/test_order.py", "from app.order import order\n\ndef test_len():\n    assert len(order()) == 3\n")
	for _, args := range [][]string{
		{"init", "-q"}, {"config", "user.email", "t@example.com"}, {"config", "user.name", "t"},
		{"add", "-A"}, {"commit", "-qm", "base"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	writeFile(t, dir, "tests/test_order.py", "from app.order import order\n\ndef test_len():\n    assert len(order()) == 3\n\n"+
		"def test_sorted():\n    assert order() == 'abc'\n\ndef test_letters():\n    assert set(order()) == set('abc')\n")
	rep, err := RedCheck(dir, "", 2*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"tests/test_order.py::test_sorted"}; !reflect.DeepEqual(rep.Red, want) {
		t.Fatalf("red = %v, want %v (pins nothing: %v)", rep.Red, want, rep.PinsNothing)
	}
	if want := []string{"tests/test_order.py::test_letters"}; !reflect.DeepEqual(rep.PinsNothing, want) {
		t.Fatalf("pins nothing = %v, want %v", rep.PinsNothing, want)
	}
}
