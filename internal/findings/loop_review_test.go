package findings_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alesierraalta/tsp/internal/findings"
)

// git trusts a file's stat only when the file is older than its index. A snapshot taken in a later second than the
// index was written must still see an edit that kept the size and landed in the index's own second, or the loop
// would read a fix as no change at all.
func TestASnapshotSeesAnEditMadeInTheSecondTheIndexWasWritten(t *testing.T) {
	if testing.Short() {
		t.Skip("real git")
	}
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
		if got := command(t, d, "git", "show", tree+":a.go"); !strings.Contains(got, "return 1") {
			t.Fatalf("the snapshot missed the edit:\n%s", got)
		}
		return
	}
	t.Skip("could not fit the write, the add and the edit inside one second")
}

// Untracked files enter a snapshot only when the loop reads them: source, tests and test data. A credentials file
// never reaches the object store, and a file nobody can read does not break the record.
func TestASnapshotTakesOnlyTheUntrackedFilesTheLoopReads(t *testing.T) {
	d := repo(t)
	put(t, d, "creds.txt", "token=secret-value\n")
	put(t, d, "extra.go", "package sample\nfunc Extra() int { return 1 }\n")
	put(t, d, "testdata/case.txt", "input\n")
	put(t, d, "locked.log", "noise\n")
	locked := filepath.Join(d, "locked.log")
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(locked, 0600) })
	put(t, d, "value_test.go", redTest)
	record(t, d)
	tree := storedLoop(t).Rounds[0].End
	names := strings.Fields(command(t, d, "git", "ls-tree", "-r", "--name-only", tree))
	if want := []string{"extra.go", "go.mod", "testdata/case.txt", "value.go", "value_test.go"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("snapshot holds %v, want %v", names, want)
	}
	secret := command(t, d, "git", "hash-object", "creds.txt")
	if out, err := exec.Command("git", "-C", d, "cat-file", "-e", secret).CombinedOutput(); err == nil {
		t.Fatalf("the credentials file reached the object store: %s", out)
	}
}

// A loop whose snapshots git gc removed cannot be followed: next says so, and the next record starts a new loop.
func TestALoopWhoseSnapshotsGitRemovedStartsOver(t *testing.T) {
	d := openRoundOne(t)
	put(t, d, "value.go", fixedValue)
	if a := next(t, d, 1); a.Next != "tsp" {
		t.Fatalf("answer: %+v", a)
	}
	command(t, d, "git", "gc", "--prune=now", "--quiet")
	gone := "the TSP loop's snapshots are gone (git gc removed them): run the TSP flow, then `tsp findings record` to start a new loop"
	if a := next(t, d, 2); a.Next != "error" || a.Reason != gone {
		t.Fatalf("answer: %+v", a)
	}
	out, code := findings.Record(d, "", wait)
	want := "a new TSP loop starts: git gc removed the snapshots of the one that was open\nF1 recorded value_test.go example.invalid/sample.TestPromise\n"
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
	failing := findings.Item{Status: "failing", Test: "example.invalid/sample/sub.TestSub", Detail: "fails now and is not one of the loop's findings"}
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
