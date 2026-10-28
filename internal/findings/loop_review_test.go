package findings_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alesierraalta/tsp/internal/bench"
	"github.com/alesierraalta/tsp/internal/findings"
)

// git trusts a file's stat only when the file is older than its index. A snapshot taken in a later second than the
// index was written must still see an edit that kept the size and landed in the index's own second, or the loop
// would read a fix as no change at all.
func TestASnapshotSeesAnEditMadeInTheSecondTheIndexWasWritten(t *testing.T) {
	if testing.Short() {
		t.Skip("real git")
	}
	t.Setenv("TSP_HOME", t.TempDir())
	for attempt := 0; attempt < 5; attempt++ {
		d := t.TempDir()
		command(t, d, "git", "init", "-q")
		start := time.Now().Unix()
		put(t, d, "a.go", "package a\nfunc V() int { return 0 }\n")
		command(t, d, "git", "add", "-A")
		put(t, d, "a.go", "package a\nfunc V() int { return 1 }\n")
		if time.Now().Unix() != start {
			continue
		}
		time.Sleep(time.Until(time.Unix(start+1, 0)) + 50*time.Millisecond)
		tree, err := findings.Snapshot(d)
		if err != nil {
			t.Fatal(err)
		}
		if got := snapshotCommand(t, d, "show", tree+":a.go"); !strings.Contains(got, "return 1") {
			t.Fatalf("the snapshot missed the edit:\n%s", got)
		}
		return
	}
	t.Skip("could not fit the write, the add and the edit inside one second")
}

// Snapshot contents are private even when inputs look like credentials, and unreadable inputs fail closed.
func TestASnapshotKeepsAllInputsPrivateAndFailsClosed(t *testing.T) {
	d := repo(t)
	put(t, d, "creds.txt", "token=secret-value\n")
	put(t, d, "fixtures/case.txt", "input\n")
	put(t, d, "value_test.go", redTest)
	record(t, d)
	tree := storedLoop(t).Rounds[0].End
	names := strings.Fields(snapshotCommand(t, d, "ls-tree", "-r", "--name-only", tree))
	want := []string{"creds.txt", "fixtures/case.txt", "go.mod", "value.go", "value_test.go"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("snapshot holds %v, want %v", names, want)
	}
	secret := command(t, d, "git", "hash-object", "creds.txt")
	if out, err := exec.Command("git", "-C", d, "cat-file", "-e", secret).CombinedOutput(); err == nil {
		t.Fatalf("credentials published: %s", out)
	}
	locked := filepath.Join(d, "locked.log")
	put(t, d, "locked.log", "noise")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0600) })
	if os.Geteuid() != 0 {
		if _, err := findings.Snapshot(d); err == nil {
			t.Fatal("unreadable input silently omitted")
		}
	}
}

// Repository gc cannot prune private snapshots. Removing the private store is reported, and record starts over.
func TestPrivateSnapshotsSurviveGitGCAndReportLostState(t *testing.T) {
	d := openRoundOne(t)
	put(t, d, "value.go", fixedValue)
	if a := next(t, d, 1); a.Next != "tsp" {
		t.Fatalf("answer: %+v", a)
	}
	command(t, d, "git", "gc", "--prune=now", "--quiet")
	next(t, d, 1)
	env, err := bench.SnapshotEnv(d)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.RemoveAll(strings.TrimPrefix(env[0], "GIT_OBJECT_DIRECTORY=")); err != nil {
		t.Fatal(err)
	}
	gone := "the TSP loop's private snapshots are unavailable (removed state, or another clone at the same path): run the TSP flow, then `tsp findings record` to start a new loop"
	if a := next(t, d, 2); a.Next != "error" || a.Reason != gone {
		t.Fatalf("answer: %+v", a)
	}
	out, code := findings.Record(d, "", wait)
	want := "a new TSP loop starts: the private snapshots of the one that was open are unavailable\nF1 recorded value_test.go example.invalid/sample.TestPromise\n"
	if code != 0 || out != want {
		t.Fatalf("record exit=%d: %q want %q", code, out, want)
	}
	if loop := storedLoop(t); len(loop.Rounds) != 1 || loop.Closed || loop.Pending != "" {
		t.Fatalf("a new loop: %+v", loop)
	}
}

// A round reruns every suite it recorded, so a test in a second suite that the change broke still holds the loop.
func TestARoundRerunsEverySuiteItRecorded(t *testing.T) {
	d := repo(t)
	commit(t, d, map[string]string{"sub/sub.go": "package sub\nfunc Sub() int { return 1 }\n"})
	put(t, d, "sub/sub.go", "package sub\nfunc Sub() int { return 2 }\n")
	put(t, d, "sub/sub_test.go", "package sub\nimport \"testing\"\nfunc TestSub(t *testing.T) { if Sub() != 1 { t.Fatal(\"sub\") } }\n")
	put(t, d, "value_test.go", "package sample\nimport \"testing\"\nfunc TestZero(t *testing.T) { if Value() != 0 { t.Fatal(\"no\") } }\n")
	for _, suite := range []string{"go test -count=1 .", "go test -count=1 ./sub"} {
		if out, code := findings.Record(d, suite, wait); code != 1 || out != "no defect witnesses recorded\n" {
			t.Fatalf("record %s: %d %q", suite, code, out)
		}
	}
	failing := findings.Item{Status: "failing", Suite: "go test -count=1 ./sub", Test: "example.invalid/sample/sub.TestSub", Detail: "fails now and is not one of the loop's findings"}
	if a := next(t, d, 1); a.Next != "fix" || !reflect.DeepEqual(a.Items, []findings.Item{failing}) {
		t.Fatalf("answer: %+v", a)
	}
}

// Every record in a later round is judged against that round's fixed code, not only the first one.
func TestEveryRecordInALaterRoundIsJudgedAgainstTheFixedCode(t *testing.T) {
	d := repo(t)
	commit(t, d, map[string]string{"double.go": doubleGo})
	put(t, d, "value_test.go", redTest)
	record(t, d)
	next(t, d, 1)
	put(t, d, "value.go", fixedValue)
	put(t, d, "double.go", "package sample\nfunc Double(x int) int { return 3 * x }\n")
	if a := next(t, d, 1); a.Next != "tsp" {
		t.Fatalf("answer: %+v", a)
	}
	put(t, d, "double_test.go", doubleTest)
	if out, code := findings.Record(d, "", wait); code != 0 || out != "F2 recorded double_test.go example.invalid/sample.TestDouble\n" {
		t.Fatalf("first record: %d %q", code, out)
	}
	put(t, d, "triple_test.go", "package sample\nimport \"testing\"\n// Doubles three.\nfunc TestDoubleThree(t *testing.T) { if Double(3) != 6 { t.Fatal(\"not doubled\") } }\n")
	if out, code := findings.Record(d, "", wait); code != 0 || !strings.Contains(out, "F3 recorded triple_test.go example.invalid/sample.TestDoubleThree\n") {
		t.Fatalf("second record: %d %q", code, out)
	}
}

// The round's git diff command quotes every path, so a name with spaces or shell syntax is one path and runs nothing.
func TestTheRoundCommandQuotesEveryPath(t *testing.T) {
	d := openRoundOne(t)
	put(t, d, "value.go", fixedValue)
	odd := "odd name $(touch PWNED).js"
	put(t, d, odd, "export const odd = 1;\n")
	a := next(t, d, 1)
	if a.Next != "tsp" || a.Scope == nil || !reflect.DeepEqual(a.Scope.Paths, []string{odd, "value.go"}) {
		t.Fatalf("answer: %+v", a)
	}
	start := strings.Index(a.Message, "`env ")
	if start < 0 {
		t.Fatalf("no command in %q", a.Message)
	}
	cmd, _, _ := strings.Cut(a.Message[start+1:], "`")
	if !strings.HasSuffix(cmd, " -- 'odd name $(touch PWNED).js' 'value.go'") {
		t.Fatalf("command: %s", cmd)
	}
	out := command(t, d, "sh", "-c", cmd)
	if _, err := os.Stat(filepath.Join(d, "PWNED")); err == nil {
		t.Fatal("running the command ran a file name")
	}
	if !strings.Contains(out, odd) || !strings.Contains(out, "value.go") {
		t.Fatalf("the diff misses a path:\n%s", out)
	}
}

// A terminal state is rejudged, not blindly replayed; its rounds survive a duplicate record.
func TestATerminalAnswerIsRepeatedOverTheSameTree(t *testing.T) {
	d := openRoundOne(t)
	put(t, d, "value.go", fixedValue)
	next(t, d, 1)
	put(t, d, "more_test.go", passingTest)
	if out, code := findings.Record(d, "", wait); code != 1 {
		t.Fatalf("dry round: %d %q", code, out)
	}
	done := next(t, d, 0)
	if done.Reason != "converged after 2 rounds: 1 fixed, 0 rejected, 0 wontfix" {
		t.Fatalf("answer: %+v", done)
	}
	again := next(t, d, 0)
	if again.Review == done.Review {
		t.Fatal("done was replayed without a fresh review")
	}
	again.Review = done.Review
	if !reflect.DeepEqual(again, done) {
		t.Fatalf("fresh review changed the round: %+v, want %+v", again, done)
	}
	if out, code := findings.Record(d, "", wait); code != 1 || out != "no defect witnesses recorded\n" {
		t.Fatalf("duplicate record must be a dry round, not invent a witness: %d %q", code, out)
	}
	after := next(t, d, 0)
	after.Review = done.Review
	if !reflect.DeepEqual(after, done) {
		t.Fatalf("duplicate record changed history: %+v, want %+v", after, done)
	}
	put(t, d, "more.go", "package sample\nfunc More() int { return 2 }\n")
	if a := next(t, d, 1); a.Next != "tsp" || a.Scope == nil || !reflect.DeepEqual(a.Scope.Paths, []string{"more.go"}) {
		t.Fatalf("new code did not owe an attack: %+v", a)
	}
}

// A suite that stops building in between does not make a failing test count as failing again: it never passed.
func TestABuildFailureInBetweenIsNotARelapse(t *testing.T) {
	d := repo(t)
	commit(t, d, map[string]string{"double.go": doubleGo, "double_test.go": doubleTest})
	put(t, d, "value_test.go", redTest)
	record(t, d)
	next(t, d, 1)
	put(t, d, "value.go", fixedValue)
	broken := "package sample\nfunc Double(x int) int { return 3 * x }\n"
	put(t, d, "double.go", broken)
	if a := next(t, d, 1); a.Next != "fix" {
		t.Fatalf("the break: %+v", a)
	}
	put(t, d, "double.go", "not valid Go")
	a := next(t, d, 1)
	suite := false
	for _, it := range a.Items {
		suite = suite || it.Status == "suite"
	}
	if a.Next != "fix" || !suite {
		t.Fatalf("the build failure: %+v", a)
	}
	put(t, d, "double.go", broken)
	if a := next(t, d, 1); a.Next != "fix" {
		t.Fatalf("a test that never passed counted as failing again: %+v", a)
	}
}

// next checks that the tree it judged is still the tree when it answers: a suite that writes source while it runs
// leaves next nothing to answer, and asking again judges the tree as it is now.
func TestNextRefusesATreeThatChangedWhileItJudged(t *testing.T) {
	d := repo(t)
	put(t, d, "value_test.go", "package sample\nimport (\n\t\"os\"\n\t\"testing\"\n)\nfunc TestWrites(t *testing.T) { _ = os.WriteFile(\"gen.js\", []byte(\"export const g = 1;\\n\"), 0o644) }\n")
	if out, code := findings.Record(d, "", wait); code != 1 {
		t.Fatalf("record: %d %q", code, out)
	}
	moved := "the working tree changed while next judged it: run next again"
	if a := next(t, d, 2); a.Next != "error" || a.Reason != moved {
		t.Fatalf("answer: %+v", a)
	}
	if a := next(t, d, 1); a.Next != "tsp" || a.Scope == nil || !reflect.DeepEqual(a.Scope.Paths, []string{"gen.js"}) {
		t.Fatalf("asking again: %+v", a)
	}
}

// A witness the user kept as wontfix still fails; the loop's end names it instead of reading as every test passing.
func TestAWontfixWitnessThatStillFailsIsNamedAtTheEnd(t *testing.T) {
	d := openRoundOne(t)
	if out, code := findings.SetVerdict(d, "F1", "wontfix", "accepted for this release"); code != 0 {
		t.Fatalf("verdict: %d %q", code, out)
	}
	a := next(t, d, 0)
	if note := "kept failing by the user's wontfix: F1 value_test.go example.invalid/sample.TestPromise — accepted for this release"; a.Next != "done" || !hasNote(a, note) {
		t.Fatalf("answer: %+v", a)
	}
}

// A failing test that shares a witness's name but runs in another suite is not that witness.
func TestAWitnessCoversItsTestInItsOwnSuiteOnly(t *testing.T) {
	d := repo(t)
	put(t, d, "a_test.go", "//go:build !b\n\npackage sample\n\nimport \"testing\"\n\nfunc TestSame(t *testing.T) { if Value() != 1 { t.Fatal(\"a\") } }\n")
	put(t, d, "b_test.go", "//go:build b\n\npackage sample\n\nimport \"testing\"\n\nfunc TestSame(t *testing.T) { if Value() != 0 { t.Fatal(\"b\") } }\n")
	if out, code := findings.Record(d, "go test -count=1 .", wait); code != 0 || out != "F1 recorded a_test.go example.invalid/sample.TestSame\n" {
		t.Fatalf("record a: %d %q", code, out)
	}
	if out, code := findings.Record(d, "go test -count=1 -tags=b .", wait); code != 1 {
		t.Fatalf("record b: %d %q", code, out)
	}
	put(t, d, "value.go", fixedValue)
	failing := findings.Item{Status: "failing", Suite: "go test -count=1 -tags=b .", Test: "example.invalid/sample.TestSame", Detail: "fails now and is not one of the loop's findings"}
	if a := next(t, d, 1); a.Next != "fix" || !reflect.DeepEqual(a.Items, []findings.Item{failing}) {
		t.Fatalf("answer: %+v", a)
	}
}

// The answer carries every finding's review line, so a host can show the whole review beside the verdict.
func TestTheAnswerCarriesEveryFindingsReviewLine(t *testing.T) {
	d := openRoundOne(t)
	if a := next(t, d, 1); !reflect.DeepEqual(a.Lines, []string{promiseLine}) {
		t.Fatalf("lines: %q", a.Lines)
	}
	if a, _ := findings.Next(t.TempDir(), wait); a.Lines == nil || len(a.Lines) != 0 {
		t.Fatalf("an error answer carries lines %v", a.Lines)
	}
}
