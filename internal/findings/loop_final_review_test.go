package findings_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/alesierraalta/tsp/internal/findings"
)

// roundCommand is the diff command a tsp answer tells the agent to run, without its backquotes.
func roundCommand(t *testing.T, a findings.Answer) string {
	t.Helper()
	start := strings.Index(a.Message, "`git ")
	if start < 0 {
		t.Fatalf("no command in %q", a.Message)
	}
	cmd, _, _ := strings.Cut(a.Message[start+1:], "`")
	return cmd
}

// A file whose name is Git pathspec magic is still a path: the round's diff shows it instead of excluding it.
func TestTheRoundCommandReadsPathspecMagicLiterally(t *testing.T) {
	d := openRoundOne(t)
	put(t, d, "value.go", fixedValue)
	magic := ":(exclude)magic.js"
	put(t, d, magic, "export const magic = 1;\n")
	a := next(t, d, 1)
	if a.Next != "tsp" || a.Scope == nil || !reflect.DeepEqual(a.Scope.Paths, []string{magic, "value.go"}) {
		t.Fatalf("answer: %+v", a)
	}
	out := command(t, d, "sh", "-c", roundCommand(t, a))
	if !strings.Contains(out, "+export const magic = 1;") || !strings.Contains(out, "value.go") {
		t.Fatalf("the round's diff hides a changed path:\n%s", out)
	}
}

// An explicit --since is the round's base as the commit it names, however it was spelled: never its parent, and
// the same round when it is recorded again.
func TestAnExplicitSinceIsTheRoundsBase(t *testing.T) {
	d := repo(t)
	put(t, d, "other.go", "package sample\nfunc Other() int { return 2 }\n")
	since := commitAll(t, d, "a change before the round")
	put(t, d, "value_test.go", redTest)
	commitAll(t, d, "round one")
	for i := 1; i <= 2; i++ {
		out, code := findings.Record(d, "", "HEAD~1", wait)
		if code != 0 || out != "F1 recorded value_test.go example.invalid/sample.TestPromise\n" {
			t.Fatalf("record %d --since HEAD~1 exit=%d: %q", i, code, out)
		}
	}
	if base := storedLoop(t).Rounds[0].Base; base != since {
		t.Fatalf("round base = %s, want the --since commit %s", base, since)
	}
}

const onceOnly = "package sample\nimport \"testing\"\nvar seen int\n// Holds on its first run only.\nfunc TestOnceOnly(t *testing.T) { seen++; if seen > 1 { t.Fatal(\"failed on a later iteration\") } }\n"

// Every command a round recorded runs as it was recorded: a failure that only a later iteration shows holds the loop
// even when the newest command runs one iteration.
func TestEveryRecordedIterationCountRuns(t *testing.T) {
	d := repo(t)
	put(t, d, "once_test.go", onceOnly)
	commitAll(t, d, "a test only its first iteration passes")
	put(t, d, "value_test.go", redTest)
	commitAll(t, d, "round one")
	for _, suite := range []string{"go test -count=3 .", "go test -count=1 ."} {
		if out, code := recordSuite(t, d, suite); code != 0 {
			t.Fatalf("record %s: %d %q", suite, code, out)
		}
	}
	put(t, d, "value.go", fixedValue)
	commitAll(t, d, "fix the promise")
	a := next(t, d, 1)
	for _, it := range a.Items {
		if it.Status == "failing" && it.Suite == "go test -count=3 ." && strings.HasSuffix(it.Test, "TestOnceOnly") {
			return
		}
	}
	t.Fatalf("the three-iteration command did not run: %+v", a)
}

// A record after the fix keeps the round where it started, so the fix is still owed a focused round of its own.
func TestARecordAfterTheFixStillOwesItsRound(t *testing.T) {
	d := openRoundOne(t)
	attacked := storedLoop(t).Rounds[0].End
	put(t, d, "value.go", fixedValue)
	commitPending(t, d)
	if out, code := recordSuite(t, d, "go test -count=1 ."); code != 0 {
		t.Fatalf("record after the fix: %d %q", code, out)
	}
	if got := storedLoop(t).Rounds[0].End; got != attacked {
		t.Fatalf("round end moved to %s, want the commit the round attacked %s", got, attacked)
	}
	if a := next(t, d, 1); a.Next != "tsp" || a.Round != 2 || a.Scope == nil || !reflect.DeepEqual(a.Scope.Paths, []string{"value.go"}) {
		t.Fatalf("the fix was never attacked: %+v", a)
	}
}

// Source committed together with a later round's tests was never attacked by that round: the loop owes another.
func TestSourceCommittedWithARoundsTestsOwesAnotherRound(t *testing.T) {
	d := openRoundOne(t)
	put(t, d, "value.go", fixedValue)
	if a := next(t, d, 1); a.Next != "tsp" || a.Round != 2 {
		t.Fatalf("round two: %+v", a)
	}
	put(t, d, "value.go", fixedValue+"func AddedWithTheTests() int { return 3 }\n")
	put(t, d, "more_test.go", passingTest)
	if out, code := recordSuite(t, d, ""); code != 1 {
		t.Fatalf("round two record: %d %q", code, out)
	}
	a := next(t, d, 1)
	if a.Next != "tsp" || a.Round != 3 || a.Scope == nil || !reflect.DeepEqual(a.Scope.Paths, []string{"value.go"}) {
		t.Fatalf("the source the round never attacked was swallowed: %+v", a)
	}
}

// A recorded suite that now runs no test proves nothing passes: the end names it as not measured instead of counting
// it as clean. It does not hold the loop, because removing a rejected test can empty the suite that held it.
func TestARecordedSuiteThatRunsNoTestIsNotMeasured(t *testing.T) {
	d := repo(t)
	put(t, d, "value_test.go", "package sample\nimport \"testing\"\nfunc TestZero(t *testing.T) { if Value() != 0 { t.Fatal(\"no\") } }\n")
	commitAll(t, d, "round one")
	suite := "go test -count=1 ."
	if out, code := recordSuite(t, d, suite); code != 1 {
		t.Fatalf("record: %d %q", code, out)
	}
	command(t, d, "git", "rm", "-q", "value_test.go")
	a := next(t, d, 0)
	want := "not measured: `" + suite + "` ran no test"
	if a.Next != "done" || !hasNote(a, want) || !strings.Contains(a.Text(), want) {
		t.Fatalf("an empty run read as clean: %+v\n%s", a, a.Text())
	}
}
