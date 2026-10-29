package findings_test

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alesierraalta/tsp/internal/findings"
)

const brokenValue = "package sample\nfunc Value() int { return 0 }\n"
const doubleGo = "package sample\nfunc Double(x int) int { return 2 * x }\n"
const doubleTest = "package sample\nimport \"testing\"\n// Doubles its argument.\nfunc TestDouble(t *testing.T) { if Double(2) != 4 { t.Fatal(\"not doubled\") } }\n"

// commit writes the files and commits them: the code the loop starts from.
func commit(t *testing.T, d string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		put(t, d, name, body)
	}
	command(t, d, "git", "add", ".")
	command(t, d, "git", "commit", "-qm", "base")
}

func hasNote(a findings.Answer, prefix string) bool {
	for _, n := range a.Notes {
		if strings.HasPrefix(n, prefix) {
			return true
		}
	}
	return false
}

// Rejecting an invented witness and removing its test settles it whichever comes first.
func TestAVerdictSurvivesTheRemovalOfItsTest(t *testing.T) {
	for _, order := range []string{"verdict first", "removal first"} {
		t.Run(order, func(t *testing.T) {
			d := openRoundOne(t)
			reject := func() {
				if out, code := findings.SetVerdict(d, "F1", "rejected", "the contract promises 0"); code != 0 {
					t.Fatalf("verdict: %d %q", code, out)
				}
			}
			remove := func() {
				if err := os.Remove(filepath.Join(d, "value_test.go")); err != nil {
					t.Fatal(err)
				}
			}
			if order == "verdict first" {
				reject()
				remove()
			} else {
				remove()
				reject()
			}
			out, code := findings.Check(d, wait)
			want := "review 2 on topic: 0 fixed, 0 open, 0 new, 0 regressed\nF1 rejected value_test.go example.invalid/sample.TestPromise — the contract promises 0\n"
			if code != 0 || out != want {
				t.Fatalf("check exit=%d\n got %q\nwant %q", code, out, want)
			}
			if a := next(t, d, 0); a.Reason != "converged after 1 round: 0 fixed, 1 rejected, 0 wontfix" {
				t.Fatalf("answer: %+v", a)
			}
		})
	}
}

// Only a removed test keeps its verdict: a test that still exists but changed is judged again, as before.
func TestAVerdictOnAChangedTestIsStillDropped(t *testing.T) {
	d := openRoundOne(t)
	if out, code := findings.SetVerdict(d, "F1", "rejected", "the contract promises 0"); code != 0 {
		t.Fatalf("verdict: %d %q", code, out)
	}
	put(t, d, "value_test.go", strings.ReplaceAll(redTest, "broken", "still broken"))
	out, code := findings.Check(d, wait)
	want := "review 2 on topic: 0 fixed, 1 open, 0 new, 0 regressed\nF1 open value_test.go example.invalid/sample.TestPromise — Keeps the promise. (verdict dropped: test changed)\n"
	if code != 1 || out != want {
		t.Fatalf("check exit=%d\n got %q\nwant %q", code, out, want)
	}
}

func TestAFindingThatRegressesTwiceStopsTheLoop(t *testing.T) {
	d := openRoundOne(t)
	put(t, d, "value.go", fixedValue)
	next(t, d, 1)
	put(t, d, "value.go", brokenValue)
	if a := next(t, d, 1); a.Next != "fix" || len(a.Items) != 1 || a.Items[0].Status != "regressed" {
		t.Fatalf("first regression: %+v", a)
	}
	put(t, d, "value.go", fixedValue)
	next(t, d, 1)
	put(t, d, "value.go", brokenValue)
	a := next(t, d, 3)
	reason := "F1 regressed twice: the fixes keep undoing each other"
	line := "F1 regressed value_test.go example.invalid/sample.TestPromise — Keeps the promise."
	if a.Next != "stop" || a.Reason != reason || a.Message != reason+" — the loop needs the user's decision\n"+line || a.Text() != "stop: "+reason+" — the loop needs the user's decision\n"+a.Review+"\n"+line+"\n" {
		t.Fatalf("answer: %+v\n%q", a, a.Text())
	}
	if !storedLoop(t).Closed {
		t.Fatal("a stopped loop stays open")
	}
}

// Two fixes in a row that settle nothing stop the loop; asking again without changing anything is no attempt.
func TestTwoFixesThatSettleNothingStopTheLoop(t *testing.T) {
	d := openRoundOne(t)
	put(t, d, "other.go", "package sample\nfunc Other() int { return 1 }\n")
	if a := next(t, d, 1); a.Next != "fix" {
		t.Fatalf("first attempt: %+v", a)
	}
	if a := next(t, d, 1); a.Next != "fix" {
		t.Fatalf("a re-query counted as an attempt: %+v", a)
	}
	put(t, d, "other.go", "package sample\nfunc Other() int { return 2 }\n")
	a := next(t, d, 3)
	reason := "no progress: 1 item stayed unsettled after two fixes"
	if a.Next != "stop" || a.Reason != reason || a.Message != reason+" — the loop needs the user's decision\n"+promiseLine {
		t.Fatalf("answer: %+v", a)
	}
}

// A test the fix breaks is the next fix's to repair.
func TestATestTheFixBreaksMustBeFixed(t *testing.T) {
	d := repo(t)
	commit(t, d, map[string]string{"double.go": doubleGo, "double_test.go": doubleTest})
	put(t, d, "value_test.go", redTest)
	record(t, d)
	next(t, d, 1)
	put(t, d, "value.go", fixedValue)
	put(t, d, "double.go", "package sample\nfunc Double(x int) int { return 3 * x }\n")
	a := next(t, d, 1)
	item := findings.Item{Status: "failing", Suite: "go test -count=1 .", Test: "example.invalid/sample.TestDouble", Detail: "fails now and is not one of the loop's findings"}
	if a.Next != "fix" || !reflect.DeepEqual(a.Items, []findings.Item{item}) ||
		!strings.HasPrefix(a.Message, "failing example.invalid/sample.TestDouble — fails now and is not one of the loop's findings\n") ||
		!strings.Contains(a.Message, "A failing test is not a finding, and the loop does not converge while it fails: fix the source that makes it fail. Never edit or remove that test; when it cannot pass otherwise, stop and say so, because that decision is the user's.") {
		t.Fatalf("answer: %+v", a)
	}
	put(t, d, "double.go", doubleGo)
	if b := next(t, d, 1); b.Next != "tsp" {
		t.Fatalf("after repairing it: %+v", b)
	}
}

// A test the fixes make pass and then break again stops the loop: the fixes keep undoing each other.
func TestATestThatFailsAgainAfterAFixStopsTheLoop(t *testing.T) {
	d := repo(t)
	commit(t, d, map[string]string{"double.go": doubleGo, "double_test.go": doubleTest})
	put(t, d, "value_test.go", redTest)
	record(t, d)
	next(t, d, 1)
	put(t, d, "value.go", fixedValue)
	put(t, d, "double.go", "package sample\nfunc Double(x int) int { return 3 * x }\n")
	if a := next(t, d, 1); a.Next != "fix" {
		t.Fatalf("the break: %+v", a)
	}
	put(t, d, "double.go", doubleGo)
	if a := next(t, d, 1); a.Next != "tsp" {
		t.Fatalf("the repair: %+v", a)
	}
	put(t, d, "more_test.go", passingTest)
	if out, code := recordSuite(t, d, ""); code != 1 {
		t.Fatalf("dry round: %d %q", code, out)
	}
	put(t, d, "double.go", "package sample\nfunc Double(x int) int { return 3 * x }\n")
	a := next(t, d, 3)
	reason := "example.invalid/sample.TestDouble failed again after a fix had made it pass: the fixes keep undoing each other"
	if a.Next != "stop" || a.Reason != reason || a.Message != reason+" — the loop needs the user's decision\nfailing example.invalid/sample.TestDouble — fails now and is not one of the loop's findings" {
		t.Fatalf("answer: %+v", a)
	}
	if !storedLoop(t).Closed {
		t.Fatal("a stopped loop stays open")
	}
}

// A test that already failed before the loop holds it too: the loop never calls a suite with a red test converged,
// because the loop cannot tell a failure older than the change from one the change caused.
func TestAFailingTestFromBeforeTheLoopStillHoldsIt(t *testing.T) {
	d := repo(t)
	commit(t, d, map[string]string{
		"old.go":      "package sample\nfunc Old() int { return 0 }\n",
		"old_test.go": "package sample\nimport \"testing\"\nfunc TestOld(t *testing.T) { if Old() != 1 { t.Fatal(\"old\") } }\n",
	})
	put(t, d, "value_test.go", redTest)
	record(t, d)
	failing := findings.Item{Status: "failing", Suite: "go test -count=1 .", Test: "example.invalid/sample.TestOld", Detail: "fails now and is not one of the loop's findings"}
	if a := next(t, d, 1); a.Next != "fix" || len(a.Items) != 2 || a.Items[0].ID != "F1" || !reflect.DeepEqual(a.Items[1], failing) || len(a.Notes) != 0 {
		t.Fatalf("answer: %+v", a)
	}
	put(t, d, "value.go", fixedValue)
	if a := next(t, d, 1); a.Next != "fix" || !reflect.DeepEqual(a.Items, []findings.Item{failing}) {
		t.Fatalf("a red test was left behind: %+v", a)
	}
	put(t, d, "old.go", "package sample\nfunc Old() int { return 1 }\n")
	if a := next(t, d, 1); a.Next != "tsp" || a.Scope == nil || !reflect.DeepEqual(a.Scope.Paths, []string{"old.go", "value.go"}) {
		t.Fatalf("once every test passes: %+v", a)
	}
}

// A suite that no longer runs cannot vouch for anything: making it run again is the next fix.
func TestASuiteThatCannotRunMustBeFixed(t *testing.T) {
	d := repo(t)
	put(t, d, "value_test.go", "package sample\nimport \"testing\"\nfunc TestZero(t *testing.T) { if Value() != 0 { t.Fatal(\"no\") } }\n")
	if out, code := recordSuite(t, d, ""); code != 1 {
		t.Fatalf("record: %d %q", code, out)
	}
	put(t, d, "value.go", "not valid Go")
	a := next(t, d, 1)
	if a.Next != "fix" || len(a.Items) != 1 || a.Items[0].Status != "suite" || a.Items[0].Test != "go test -count=1 ." || !strings.HasPrefix(a.Items[0].Detail, "did not run test by test") ||
		!strings.Contains(a.Message, "A suite that did not run test by test is fixed first: nothing it holds can be judged until it does.") {
		t.Fatalf("answer: %+v", a)
	}
}

// Changed source with no test beside it is the check's debt: the loop cannot converge over it.
func TestUntestedSourceHoldsTheLoopAtTheCheck(t *testing.T) {
	d := repo(t)
	put(t, d, "triple.go", "package sample\nfunc Triple(x int) int { return 3 * x }\n")
	if out, code := recordSuite(t, d, ""); code != 1 || out != "no defect witnesses recorded\n" {
		t.Fatalf("record: %d %q", code, out)
	}
	a := next(t, d, 1)
	if a.Next != "fix" || len(a.Items) != 1 || a.Items[0].Status != "check" || !strings.HasPrefix(a.Items[0].Detail, "no test was added or changed with it") ||
		!strings.Contains(a.Message, "A check, quality or arch item is fixed in the code or the tests it names.") {
		t.Fatalf("answer: %+v", a)
	}
	put(t, d, "triple_test.go", "package sample\nimport \"testing\"\nfunc TestTriple(t *testing.T) { if Triple(2) != 6 { t.Fatal(\"no\") } }\n")
	if b := next(t, d, 0); b.Next != "done" {
		t.Fatalf("with its test: %+v", b)
	}
}

// Code the change made too complex holds the loop at the quality probe, and its refactor owes another round.
func TestAComplexChangeHoldsTheLoopAtTheQualityProbe(t *testing.T) {
	d := repo(t)
	var b strings.Builder
	b.WriteString("package sample\nfunc Grade(x int) int {\n")
	for i := 0; i < 16; i++ {
		fmt.Fprintf(&b, "\tif x == %d {\n\t\treturn %d\n\t}\n", i, i*2)
	}
	b.WriteString("\treturn -1\n}\n")
	put(t, d, "grade.go", b.String())
	put(t, d, "grade_test.go", "package sample\nimport \"testing\"\nfunc TestValueStillZero(t *testing.T) { if Value() != 0 { t.Fatal(\"no\") } }\n")
	if out, code := recordSuite(t, d, ""); code != 1 {
		t.Fatalf("record: %d %q", code, out)
	}
	a := next(t, d, 1)
	if a.Next != "fix" || len(a.Items) == 0 {
		t.Fatalf("answer: %+v", a)
	}
	for _, it := range a.Items {
		if it.Status != "quality" || it.File != "grade.go" {
			t.Fatalf("item: %+v in %+v", it, a)
		}
	}
	put(t, d, "grade.go", "package sample\nfunc Grade(x int) int {\n\tif x < 0 || x > 15 {\n\t\treturn -1\n\t}\n\treturn 2 * x\n}\n")
	if c := next(t, d, 1); c.Next != "tsp" {
		t.Fatalf("a refactor needs a round of its own: %+v", c)
	}
}

// An import edge the repository refuses holds the loop at the architecture probe.
func TestAForbiddenImportHoldsTheLoopAtTheArchProbe(t *testing.T) {
	d := repo(t)
	commit(t, d, map[string]string{
		"go.mod":         "module example.com/m\n\ngo 1.22\n",
		".tsp-arch.json": `{"layers":[{"name":"svc","paths":["svc/"],"imports":["example.com/m/svc"]},{"name":"cmd","paths":["cmd/"],"imports":["example.com/m/cmd"]}],"forbid":[{"from":"svc","to":"cmd","why":"a service must not reach into the command line"}]}`,
		"svc/svc.go":     "package svc\n\n// Name is a service name.\nconst Name = \"svc\"\n",
		"cmd/root.go":    "package cmd\n\n// Root is a command.\nconst Root = \"root\"\n",
	})
	put(t, d, "svc/svc.go", "package svc\n\nimport \"example.com/m/cmd\"\n\n// Name is a service name.\nconst Name = \"svc\" + cmd.Root\n")
	put(t, d, "svc/svc_test.go", "package svc\nimport \"testing\"\nfunc TestName(t *testing.T) { if Name != \"svcroot\" { t.Fatal(Name) } }\n")
	if out, code := recordSuite(t, d, ""); code > 1 {
		t.Fatalf("record: %d %q", code, out)
	}
	a := next(t, d, 1)
	found := false
	for _, it := range a.Items {
		found = found || (it.Status == "arch" && it.File == "svc/svc.go")
	}
	if a.Next != "fix" || !found {
		t.Fatalf("answer: %+v", a)
	}
}

// A gate that cannot measure is named as not measured, never as clean, and never holds the loop.
func TestAGateThatCannotMeasureIsNamed(t *testing.T) {
	d := repo(t)
	put(t, d, "value_test.go", "package sample\nimport \"testing\"\nfunc TestZero(t *testing.T) { if Value() != 0 { t.Fatal(\"no\") } }\n")
	if out, code := recordSuite(t, d, ""); code != 1 {
		t.Fatalf("record: %d %q", code, out)
	}
	a := next(t, d, 0)
	if a.Next != "done" || !hasNote(a, "not measured: probe arch: ") || !strings.Contains(a.Text(), "\nnot measured: probe arch: ") {
		t.Fatalf("answer: %+v\n%s", a, a.Text())
	}
}
