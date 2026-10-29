package findings_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alesierraalta/tsp/internal/findings"
)

func TestPendingCannotHideAFailingTestAfterSourceIsReverted(t *testing.T) {
	d := openRoundOne(t)
	put(t, d, "value.go", fixedValue)
	next(t, d, 1)
	put(t, d, "value.go", "package sample\nfunc Value() int { return 0 }\n")
	put(t, d, "other_test.go", "package sample\nimport \"testing\"\nfunc TestOther(t *testing.T){t.Fatal(\"still red\")}\n")
	// Close the original witness without letting its debt mask the independent failure.
	if _, code := findings.SetVerdict(d, "F1", "rejected", "not promised"); code != 0 {
		t.Fatal(code)
	}
	if err := os.Remove(filepath.Join(d, "value_test.go")); err != nil {
		t.Fatal(err)
	}
	commitPending(t, d)
	a, code := findings.Next(d, wait)
	if code != 1 || a.Next != "fix" || !strings.Contains(a.Text(), "TestOther") {
		t.Fatalf("unrecorded failure hidden: %d %+v", code, a)
	}
}

// A rejected witness whose test moved keeps its verdict and is reported where it lives now.
func TestARejectedWitnessMovedElsewhereIsReportedThere(t *testing.T) {
	d := repo(t)
	put(t, d, "value_test.go", redTest)
	if out, code := recordSuite(t, d, "go test -count=1 ."); code != 0 {
		t.Fatalf("record: %d %q", code, out)
	}
	if out, code := findings.SetVerdict(d, "F1", "rejected", "not promised"); code != 0 {
		t.Fatalf("verdict: %d %q", code, out)
	}
	command(t, d, "git", "mv", "value_test.go", "witness_test.go")
	commitPending(t, d)
	a := next(t, d, 1)
	if a.Next != "fix" || len(a.Items) != 1 || a.Items[0].Status != "rejected" || a.Items[0].File != "witness_test.go" {
		t.Fatalf("the moved rejected test is owed where it lives now: %+v", a)
	}
	if got := storedFindings(t)[0]; got.Status != "rejected" || got.File != "witness_test.go" {
		t.Fatalf("the rejection names its new file: %+v", got)
	}
}

func TestRecordCannotSwallowAnEditAfterThePendingAttack(t *testing.T) {
	d := openRoundOne(t)
	put(t, d, "value.go", fixedValue)
	next(t, d, 1)
	if a := next(t, d, 1); a.Next != "tsp" || a.Round != 2 {
		t.Fatalf("round two: %+v", a)
	}
	put(t, d, "more_test.go", passingTest)
	if _, code := recordSuite(t, d, ""); code != 1 {
		t.Fatal(code)
	}
	put(t, d, "value.go", fixedValue+"func AddedAfterAttack() int {return 3}\n")
	commitPending(t, d)
	a, code := findings.Next(d, wait)
	if code != 1 || a.Next != "tsp" || a.Round != 3 || a.Scope == nil || a.Scope.Since == "" {
		t.Fatalf("unattacked source swallowed: %d %+v", code, a)
	}
}

func TestClosedDoneIsJudgedAgainAfterVerdictChanges(t *testing.T) {
	d := openRoundOne(t)
	if _, code := findings.SetVerdict(d, "F1", "wontfix", "human exception"); code != 0 {
		t.Fatal(code)
	}
	next(t, d, 0)
	if _, code := findings.SetVerdict(d, "F1", "clear", ""); code != 0 {
		t.Fatal(code)
	}
	a, code := findings.Next(d, wait)
	if code != 1 || a.Next != "fix" {
		t.Fatalf("stale done after verdict clear: %d %+v", code, a)
	}
}

func TestAClosedLoopRecordsANewSuiteOnTheSameTree(t *testing.T) {
	d := repo(t)
	put(t, d, "ok.test.js", "const {test}=require('node:test');test('ok',()=>{});\n")
	put(t, d, "bad.test.js", "const {test}=require('node:test');test('bad',()=>{throw Error('bad')});\n")
	if _, code := recordSuite(t, d, "node --test ok.test.js"); code != 1 {
		t.Fatal(code)
	}
	next(t, d, 0)
	if _, code := recordSuite(t, d, "node --test bad.test.js"); code != 0 {
		t.Fatal(code)
	}
	a, code := findings.Next(d, wait)
	if code != 1 || a.Next != "fix" || !strings.Contains(a.Text(), "bad") {
		t.Fatalf("new suite hidden: %d %+v", code, a)
	}
}

func TestRecoveryBelongsToTheSuiteThatFailed(t *testing.T) {
	d := repo(t)
	put(t, d, "a.test.js", "const {test}=require('node:test');test('same',()=>{throw Error('bad')});\n")
	put(t, d, "b.test.js", "const {test}=require('node:test');test('same',()=>{});\n")
	// Use pre-existing tests: their failures are not findings.
	commitPending(t, d)
	put(t, d, "value.go", fixedValue)
	commitPending(t, d)
	for _, suite := range []string{"node --test a.test.js", "node --test b.test.js"} {
		if out, code := recordSuite(t, d, suite); code != 1 {
			t.Fatalf("record %s: %d %q", suite, code, out)
		}
	}
	if a := next(t, d, 1); a.Next != "fix" {
		t.Fatal(a)
	}
	for i := 0; i < 2; i++ {
		if a := next(t, d, 1); a.Next != "fix" {
			t.Fatalf("another suite's pass counted as recovery: %+v", a)
		}
	}
}
