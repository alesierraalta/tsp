package findings_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alesierraalta/tsp/internal/findings"
)

// head is the commit the repository holds now.
func head(t *testing.T, d string) string {
	t.Helper()
	return command(t, d, "git", "rev-parse", "HEAD")
}

// commitAll commits every pending change, as the loop's contract asks before judging.
func commitAll(t *testing.T, d, msg string) string {
	t.Helper()
	command(t, d, "git", "add", ".")
	command(t, d, "git", "commit", "-qm", msg)
	return head(t, d)
}

// The loop judges commits: a working tree it cannot name cannot be recorded.
func TestRecordNeedsACommit(t *testing.T) {
	d := repo(t)
	put(t, d, "value_test.go", redTest)
	out, code := findings.Record(d, "", "", wait)
	want := "tsp findings: the working tree has uncommitted changes: commit them, then record, so the loop judges a commit\n"
	if code != 2 || out != want {
		t.Fatalf("record exit=%d: %q want %q", code, out, want)
	}
}

// A fix that was never committed gives the loop nothing to judge.
func TestAnUncommittedFixIsRefused(t *testing.T) {
	d := repo(t)
	put(t, d, "value_test.go", redTest)
	commitAll(t, d, "round one")
	record(t, d)
	next(t, d, 1)
	put(t, d, "value.go", fixedValue)
	a, code := findings.Next(d, wait)
	want := "the working tree has uncommitted changes: commit the fix, then run `tsp findings next`"
	if code != 2 {
		t.Fatalf("next exit=%d want 2: %+v", code, a)
	}
	if a.Next != "error" || a.Reason != want || a.Message != want {
		t.Fatalf("answer: %+v", a)
	}
}

// The round the loop judges is the fix commit, not a working tree behind it.
func TestACommittedFixIsTheRound(t *testing.T) {
	d := repo(t)
	put(t, d, "value_test.go", redTest)
	first := commitAll(t, d, "round one")
	record(t, d)
	a := next(t, d, 1)
	if a.Next != "fix" || a.Round != 1 {
		t.Fatalf("round one: %+v", a)
	}
	put(t, d, "value.go", fixedValue)
	fix := commitAll(t, d, "fix the promise")
	a = next(t, d, 1)
	if a.Next != "tsp" || a.Round != 2 || a.Scope == nil || a.Scope.Since != first || a.Scope.Base != fix {
		t.Fatalf("round two: %+v", a)
	}
	if a.Tree != fix {
		t.Fatalf("the round is judged on %q, want the fix commit %q", a.Tree, fix)
	}
	if rounds := storedLoop(t).Rounds; len(rounds) != 1 || rounds[0].End != first {
		t.Fatalf("recorded rounds: %+v", rounds)
	}
}

// A commit the loop recorded and the repository no longer holds ends the loop honestly.
func TestALostCommitEndsTheLoop(t *testing.T) {
	d := repo(t)
	put(t, d, "value_test.go", redTest)
	commitAll(t, d, "round one")
	record(t, d)
	command(t, d, "git", "commit", "-q", "--amend", "-m", "amended")
	command(t, d, "git", "reflog", "expire", "--expire=now", "--all")
	command(t, d, "git", "gc", "--prune=now", "--quiet")
	a := next(t, d, 2)
	want := "the TSP loop's recorded commits are gone from this repository (amended, rebased, or pruned): run the TSP flow, then `tsp findings record` to start a new loop"
	if a.Next != "error" || a.Reason != want || a.Message != want {
		t.Fatalf("answer: %+v", a)
	}
}

// Recording keeps working in a repository that holds a submodule: nothing reads its working tree.
func TestRecordKeepsWorkingWithASubmodule(t *testing.T) {
	d := repo(t)
	library := t.TempDir()
	command(t, library, "git", "init", "-b", "main")
	command(t, library, "git", "config", "user.email", "test@example.invalid")
	command(t, library, "git", "config", "user.name", "Test")
	put(t, library, "fixture.txt", "one\n")
	command(t, library, "git", "add", ".")
	command(t, library, "git", "commit", "-qm", "library")
	command(t, d, "git", "-c", "protocol.file.allow=always", "submodule", "add", library, "vendor/library")
	put(t, d, "value_test.go", redTest)
	commitAll(t, d, "round one with a submodule")
	record(t, d)
}

// Recording the same suite with more iterations keeps one finding and runs the newest command.
func TestReRecordingRefreshesTheSuiteThatRuns(t *testing.T) {
	d := repo(t)
	put(t, d, "value_test.go", redTest)
	commitAll(t, d, "round one")
	record(t, d)
	out, code := recordSuite(t, d, "go test -count=3 .")
	if code != 0 || out != "F1 recorded value_test.go example.invalid/sample.TestPromise\n" {
		t.Fatalf("record -count=3 exit=%d: %q", code, out)
	}
	if findings := storedFindings(t); len(findings) != 1 || findings[0].Suite != "go test -count=3 ." {
		t.Fatalf("findings: %+v", findings)
	}
	rounds := storedLoop(t).Rounds
	if len(rounds) != 1 || len(rounds[0].Suites) != 2 || rounds[0].Suites[1] != "go test -count=3 ." {
		t.Fatalf("the round runs both recorded commands: %+v", rounds)
	}
	if a := next(t, d, 1); len(a.Items) != 1 || a.Items[0].ID != "F1" {
		t.Fatalf("items: %+v", a.Items)
	}
}

// Two suites that differ only inside quotes are not the same command.
func TestQuotedWhitespaceStaysPartOfTheSuite(t *testing.T) {
	if findings.SameSuite("go test -run 'Test A' .", "go test -run 'Test  A' .") {
		t.Fatal("two different -run patterns read as one suite")
	}
	if !findings.SameSuite("go test -count=1 .", "go test  -count=3 .") {
		t.Fatal("the same suite rerun with another -count reads as two")
	}
}

// storedFindings reads the findings the registry holds.
func storedFindings(t *testing.T) []findings.Finding {
	t.Helper()
	data, err := os.ReadFile(registryFile(t))
	if err != nil {
		t.Fatal(err)
	}
	var stored struct {
		Findings []findings.Finding `json:"findings"`
	}
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatalf("stored findings: %s %v", data, err)
	}
	return stored.Findings
}

// The regenerated registry keeps the witness files the loop reads.
func TestACommittedRoundKeepsItsWitnessFile(t *testing.T) {
	d := repo(t)
	put(t, d, "value_test.go", redTest)
	commitAll(t, d, "round one")
	record(t, d)
	if _, err := os.Stat(filepath.Join(d, "value_test.go")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(command(t, d, "git", "status", "--porcelain"), "") {
		t.Fatal("git status needs no output")
	}
}
