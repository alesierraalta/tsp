package findings_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alesierraalta/tsp/internal/findings"
)

// R1-001, R2-001, R4-002: a snapshot must hold its own bytes, so the repository's garbage collection cannot take
// what it names.
func TestSnapshotsSurviveRepositoryGarbageCollection(t *testing.T) {
	d := repo(t)
	put(t, d, "value.go", "package sample\nfunc Value() int { return 2 }\n")
	command(t, d, "git", "add", "value.go")
	staged, err := findings.Snapshot(d)
	if err != nil {
		t.Fatal(err)
	}
	put(t, d, "value.go", "package sample\nfunc Value() int { return 3 }\n")
	command(t, d, "git", "add", "value.go")
	command(t, d, "git", "gc", "--prune=now", "--quiet")
	if got := snapshotCommand(t, d, "show", staged+":value.go"); !strings.Contains(got, "return 2") {
		t.Fatalf("the repository's garbage collection took the snapshot: %q", got)
	}
}

// R1-002: no clean filter ever receives the bytes a snapshot captures, so a filter cannot copy them outside TSP's
// private state.
func TestCleanFiltersNeverReceiveCapturedInputs(t *testing.T) {
	d := repo(t)
	marker := filepath.Join(t.TempDir(), "leaked")
	command(t, d, "git", "config", "filter.leak.clean", "sh -c 'cat > "+marker+" && cat'")
	put(t, d, ".gitattributes", "*.secret filter=leak\n")
	put(t, d, "credential.secret", "private token\n")
	if _, err := findings.Snapshot(d); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(marker); err == nil {
		t.Fatalf("a clean filter received captured bytes: %q", data)
	}
}

// R1-003, R3-assume-unchanged-snapshot, R4-001: an index flag cannot hide the source a fix changed, so the round the
// fix owes is never skipped.
func TestAssumeUnchangedCannotHideAFixedSource(t *testing.T) {
	d := openRoundOne(t)
	command(t, d, "git", "update-index", "--assume-unchanged", "value.go")
	put(t, d, "value.go", fixedValue)
	a, code := findings.Next(d, wait)
	if code != 1 || a.Next != "tsp" {
		t.Fatalf("a fix under assume-unchanged was not attacked: exit=%d %+v", code, a)
	}
}

// R1-004: TSP's own state directory is bookkeeping, never an input, so it cannot make every snapshot differ from the
// last one, even when it legitimately sits inside the worktree.
func TestStateDirectoryInsideTheWorktreeIsNotAnInput(t *testing.T) {
	d := repo(t)
	t.Setenv("TSP_HOME", filepath.Join(d, "state"))
	put(t, d, "value_test.go", redTest)
	if out, code := findings.Record(d, "", wait); code != 0 {
		t.Fatalf("record exit=%d: %s", code, out)
	}
	first, err := findings.Snapshot(d)
	if err != nil {
		t.Fatal(err)
	}
	second, err := findings.Snapshot(d)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("the loop's own state became an input: %s != %s", first, second)
	}
}

// R2-002, R3-submodule-input-snapshot: what a submodule's working tree holds is not captured, so a dirty submodule is
// refused instead of reading as an unchanged input.
func TestADirtySubmoduleFailsClosed(t *testing.T) {
	d := repo(t)
	library := t.TempDir()
	command(t, library, "git", "init", "-b", "main")
	command(t, library, "git", "config", "user.email", "test@example.invalid")
	command(t, library, "git", "config", "user.name", "Test")
	put(t, library, "fixture.txt", "kept\n")
	command(t, library, "git", "add", ".")
	command(t, library, "git", "commit", "-qm", "base")
	command(t, d, "git", "-c", "protocol.file.allow=always", "submodule", "add", library, "vendor/library")
	command(t, d, "git", "commit", "-qm", "add the submodule")
	put(t, filepath.Join(d, "vendor/library"), "fixture.txt", "changed inside the submodule\n")
	if _, err := findings.Snapshot(d); err == nil || !strings.Contains(err.Error(), "submodule") {
		t.Fatalf("a dirty submodule read as unchanged: %v", err)
	}
}

// R2-003, R3-equivalent-suite-verdict: the same suite spelled with another -count flag refreshes its finding instead
// of allocating one a wontfix would not cover.
func TestAnEquivalentSuiteRefreshesTheSameFinding(t *testing.T) {
	d := repo(t)
	put(t, d, "value_test.go", redTest)
	want := "F1 recorded value_test.go example.invalid/sample.TestPromise\n"
	out, code := findings.Record(d, "go test -count=1 .", wait)
	if code != 0 || out != want {
		t.Fatalf("record exit=%d: %q", code, out)
	}
	out, code = findings.Record(d, "go test -count=3 .", wait)
	if code != 0 || out != want {
		t.Fatalf("an equivalent suite allocated another finding: exit=%d %q", code, out)
	}
}

// R3-first-round-base-moves: the first round keeps the commit its own record pinned, so work committed between
// records cannot swallow the witnesses it already recorded.
func TestTheFirstRoundKeepsItsBaseWhenTheWorkIsCommitted(t *testing.T) {
	d := repo(t)
	put(t, d, "value_test.go", redTest)
	if out, code := findings.Record(d, "", wait); code != 0 {
		t.Fatalf("record exit=%d: %s", code, out)
	}
	command(t, d, "git", "add", ".")
	command(t, d, "git", "commit", "-qm", "wip")
	out, code := findings.Record(d, "", wait)
	if code != 0 || out != "F1 recorded value_test.go example.invalid/sample.TestPromise\n" {
		t.Fatalf("a commit between records moved the round's base: exit=%d %q", code, out)
	}
}

// R3-normalized-input-snapshot: the snapshot holds the bytes a test reads, so a line ending Git would normalize away
// is still a change.
func TestLineEndingsArePartOfTheSnapshot(t *testing.T) {
	d := repo(t)
	put(t, d, ".gitattributes", "*.txt text\n")
	command(t, d, "git", "config", "core.autocrlf", "false")
	put(t, d, "fixture.txt", "alpha\r\nbeta\r\n")
	first, err := findings.Snapshot(d)
	if err != nil {
		t.Fatal(err)
	}
	put(t, d, "fixture.txt", "alpha\nbeta\n")
	second, err := findings.Snapshot(d)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("the snapshot hides a line-ending change Git normalizes away")
	}
}

// R4-003: a closed loop still needs its snapshots, so losing them starts a new loop instead of failing against a base
// nobody can read.
func TestAClosedLoopStillNeedsItsSnapshots(t *testing.T) {
	d := repo(t)
	put(t, d, "value_test.go", redTest)
	if out, code := findings.Record(d, "", wait); code != 0 {
		t.Fatalf("record exit=%d: %s", code, out)
	}
	if a := next(t, d, 1); a.Next != "fix" {
		t.Fatalf("not a fix: %+v", a)
	}
	put(t, d, "value.go", fixedValue)
	if a := next(t, d, 1); a.Next != "tsp" || a.Round != 2 {
		t.Fatalf("the fix owes an attack round: %+v", a)
	}
	if out, code := findings.Record(d, "", wait); code != 1 || !strings.Contains(out, "no defect witnesses recorded") {
		t.Fatalf("dry round exit=%d: %q", code, out)
	}
	if a := next(t, d, 0); a.Next != "done" {
		t.Fatalf("not converged on the fixed code: %+v", a)
	}
	if err := os.RemoveAll(filepath.Join(os.Getenv("TSP_HOME"), "snapshots")); err != nil {
		t.Fatal(err)
	}
	out, code := findings.Record(d, "", wait)
	if code != 0 || !strings.Contains(out, "a new TSP loop starts") {
		t.Fatalf("a closed loop kept its missing snapshots: exit=%d %q", code, out)
	}
}

// R4-004: the commit a round judges is pinned before the suite runs, so a commit made while it runs is refused rather
// than becoming the round's base.
func TestACommitMovingDuringJudgingIsRefused(t *testing.T) {
	d := repo(t)
	put(t, d, "value_test.go", redTest)
	findings.SetJudgedCommitHook(func(top string) {
		command(t, top, "git", "-c", "user.email=tsp@example.invalid", "-c", "user.name=tsp", "commit", "-q", "--allow-empty", "-m", "during")
	})
	defer findings.SetJudgedCommitHook(nil)
	out, code := findings.Record(d, "", wait)
	if code != 2 || out != "tsp findings: the commit changed while the TSP loop judged the round: record it again\n" {
		t.Fatalf("a commit during judging was accepted: exit=%d %q", code, out)
	}
}
