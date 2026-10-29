package findings_test

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/alesierraalta/tsp/internal/findings"
)

// A rejected witness is not reopened by a different test that carries its name: the user's verdict stays.
func TestASameNamedTestElsewhereKeepsTheVerdict(t *testing.T) {
	d := repo(t)
	put(t, d, "value_test.go", redTest)
	if out, code := recordSuite(t, d, "go test -count=1 ./..."); code != 0 {
		t.Fatalf("record: %d %q", code, out)
	}
	if out, code := findings.SetVerdict(d, "F1", "rejected", "the promise was invented"); code != 0 {
		t.Fatalf("verdict: %d %q", code, out)
	}
	command(t, d, "git", "rm", "-q", "value_test.go")
	put(t, d, "other_test.go", "package sample\n\nimport \"testing\"\n\nfunc TestPromise(t *testing.T) {\n\tif Value() != 2 {\n\t\tt.Fatal(\"another test, same name\")\n\t}\n}\n")
	a := next(t, d, 1)
	for _, it := range a.Items {
		if it.ID == "F1" {
			t.Fatalf("the user's verdict was reopened: %+v", a.Items)
		}
	}
	if got := storedFindings(t)[0]; got.Status != "rejected" || got.Verdict == nil {
		t.Fatalf("stored finding = %+v, want the rejection kept", got)
	}
	found := false
	for _, it := range a.Items {
		if it.Status == "failing" && strings.Contains(it.Test, "TestPromise") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the different failing test is still a failure: %+v", a.Items)
	}
}

// A rejected finding is settled when the test it was recorded for is gone, even if another test takes the name.
func TestARejectionSettlesOnItsOwnFileNotItsName(t *testing.T) {
	d := repo(t)
	put(t, d, "value_test.go", redTest)
	if out, code := recordSuite(t, d, "go test -count=1 ./..."); code != 0 {
		t.Fatalf("record: %d %q", code, out)
	}
	if out, code := findings.SetVerdict(d, "F1", "rejected", "the promise was invented"); code != 0 {
		t.Fatalf("verdict: %d %q", code, out)
	}
	command(t, d, "git", "rm", "-q", "value_test.go")
	commitPending(t, d)
	if a := next(t, d, 0); a.Next != "done" {
		t.Fatalf("a removed rejection settles the loop: %+v", a)
	}
	put(t, d, "other_test.go", "package sample\n\nimport \"testing\"\n\nfunc TestPromise(t *testing.T) {\n\tif Value() != 2 {\n\t\tt.Fatal(\"another test, same name\")\n\t}\n}\n")
	a := next(t, d, 1)
	for _, it := range a.Items {
		if it.ID == "F1" {
			t.Fatalf("the rejection is still owed: %+v", a.Items)
		}
	}
	if len(a.Items) != 1 || a.Items[0].Status != "failing" {
		t.Fatalf("the homonym is a failure of its own: %+v", a.Items)
	}
	put(t, d, "value.go", "package sample\n\n// Value returns the promise.\nfunc Value() int { return 2 }\n")
	commitPending(t, d)
	if out, code := recordSuite(t, d, "go test -count=1 ./..."); code != 0 {
		t.Fatalf("record the fixed round: %d %q", code, out)
	}
	if a := next(t, d, 1); a.Next != "tsp" {
		t.Fatalf("the fix is owed its own round: %+v", a)
	}
	if out, code := recordSuite(t, d, "go test -count=1 ./..."); code != 1 {
		t.Fatalf("the round over the fix finds nothing new: %d %q", code, out)
	}
	b := next(t, d, 0)
	if b.Next != "done" {
		t.Fatalf("the loop converges once the homonym passes: %+v", b)
	}
	for _, it := range b.Items {
		if it.ID == "F1" {
			t.Fatalf("the rejection reappeared: %+v", b.Items)
		}
	}
}

// A converged answer carries an empty list, never null.
func TestAConvergedAnswerCarriesAnEmptyList(t *testing.T) {
	d := openRoundOne(t)
	put(t, d, "value.go", fixedValue)
	if a := next(t, d, 1); a.Next != "tsp" {
		t.Fatalf("the fix is owed its own round: %+v", a)
	}
	if out, code := recordSuite(t, d, "go test -count=1 ."); code != 1 {
		t.Fatalf("the round over the fix finds nothing new: %d %q", code, out)
	}
	a := next(t, d, 0)
	if a.Next != "done" {
		t.Fatalf("the loop converges: %+v", a)
	}
	raw, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"items":[]`) {
		t.Fatalf("converged items = %s, want []", raw)
	}
}

// A witness that leaves the suite between two failures is not a second regression: nothing passed in between.
func TestAWitnessLeavingTheSuiteIsNotASecondRegression(t *testing.T) {
	d := repo(t)
	put(t, d, "value_test.go", redTest)
	if out, code := recordSuite(t, d, "go test -count=1 ."); code != 0 {
		t.Fatalf("record: %d %q", code, out)
	}
	next(t, d, 1)
	put(t, d, "value.go", fixedValue)
	commitPending(t, d)
	recordSuite(t, d, "go test -count=1 .")
	next(t, d, 1)
	put(t, d, "value.go", brokenValue)
	commitPending(t, d)
	recordSuite(t, d, "go test -count=1 .")
	next(t, d, 1)
	if got := storedFindings(t)[0]; got.Regressions != 1 || got.Status != "regressed" {
		t.Fatalf("first regression = %+v, want one regression", got)
	}
	put(t, d, "value_test.go", "//go:build ignore\n\n"+redTest)
	put(t, d, "other_test.go", "package sample\n\nimport \"testing\"\n\nfunc TestOther(t *testing.T) { t.Fatal(\"other\") }\n")
	commitPending(t, d)
	recordSuite(t, d, "go test -count=1 .")
	next(t, d, 1)
	command(t, d, "git", "rm", "-q", "other_test.go")
	put(t, d, "value_test.go", redTest)
	commitPending(t, d)
	recordSuite(t, d, "go test -count=1 .")
	a := next(t, d, 1)
	if got := storedFindings(t)[0]; got.Regressions != 1 {
		t.Fatalf("the gap counted a regression: %+v", got)
	}
	if a.Next == "stop" {
		t.Fatalf("one observed regression must not stop the loop: %s", a.Message)
	}
}

// A test that borrows a rejected test's name is a finding of its own; the user's verdict stays where it was made.
func TestABorrowedNameDoesNotInheritTheRejection(t *testing.T) {
	d := repo(t)
	put(t, d, "value_test.go", redTest)
	if out, code := recordSuite(t, d, "go test -count=1 ."); code != 0 {
		t.Fatalf("record: %d %q", code, out)
	}
	if out, code := findings.SetVerdict(d, "F1", "rejected", "not promised"); code != 0 {
		t.Fatalf("verdict: %d %q", code, out)
	}
	command(t, d, "git", "rm", "-q", "value_test.go")
	commitPending(t, d)
	put(t, d, "other_test.go", "package sample\n\nimport \"testing\"\n\nfunc TestPromise(t *testing.T) {\n\tif Value() != 3 {\n\t\tt.Fatal(\"a different promise\")\n\t}\n}\n")
	a := next(t, d, 1)
	for _, it := range a.Items {
		if it.File == "other_test.go" && it.ID == "F1" {
			t.Fatalf("the borrowed name inherited the rejection: %+v", a.Items)
		}
	}
	stored := storedFindings(t)
	if stored[0].Status != "rejected" || stored[0].File != "value_test.go" {
		t.Fatalf("the rejection moved: %+v", stored[0])
	}
	failing := 0
	for _, it := range a.Items {
		if it.Status == "failing" && it.ID == "" && strings.Contains(it.Test, "TestPromise") {
			failing++
		}
	}
	if failing != 1 {
		t.Fatalf("the borrow is a plain failure until it is recorded: %+v", a.Items)
	}
}

// A round whose suite could not collect every input judged around what it could not reach: the
// change that never got a verdict keeps the loop open instead of letting it claim convergence.
func TestAChangedFileThatNeverGotAVerdictKeepsTheLoopOpen(t *testing.T) {
	if out, err := exec.Command("python3", "-m", "pytest", "--version").CombinedOutput(); err != nil {
		t.Skipf("pytest unavailable: %s", out)
	}
	d := repo(t)
	put(t, d, "calc.py", "def twice(n):\n    return n * 2\n")
	put(t, d, "tests/test_ok.py", "import calc\n\ndef test_ok():\n    assert calc.twice(1) == 2\n")
	put(t, d, "tests/test_broken.py", "import modulo_que_no_existe\n")
	commitPending(t, d)
	put(t, d, "tests/test_twice.py", "import calc\n\ndef test_twice():\n    assert calc.twice(2) == 5\n")
	commitPending(t, d)
	if out, code := recordSuite(t, d, "python3 -m pytest"); code != 0 {
		t.Fatalf("record: %d %q", code, out)
	}
	loop := storedLoop(t)
	if len(loop.Rounds) == 0 || len(loop.Rounds[0].Uncollected) == 0 {
		t.Fatalf("the round lost the files that broke collection: %+v", loop.Rounds)
	}
	if got := loop.Rounds[0].Uncollected[0]; !strings.HasPrefix(got, "tests/test_broken.py: ") {
		t.Fatalf("uncollected=%q", got)
	}
	a, code := findings.Next(d, wait)
	if code == 0 && a.Next == "" {
		t.Fatalf("next claimed convergence over a change no suite judged: %s", a.Text())
	}
	if !strings.Contains(a.Text(), "not measured: no recorded suite collected tests/test_broken.py: ") {
		t.Fatalf("the answer does not say what was not measured: %s", a.Text())
	}
	unjudged := false
	for _, it := range a.Items {
		if it.Status == "unjudged" {
			unjudged = true
			if it.File == "" {
				t.Fatalf("the unjudged item carries no subject: %s", a.Text())
			}
		}
	}
	if !unjudged {
		t.Fatalf("no unjudged item in %s", a.Text())
	}
}
