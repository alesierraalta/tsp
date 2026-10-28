package findings_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alesierraalta/tsp/internal/findings"
)

// Snapshot names are literal, and no working-tree contents enter the repository's object store.
func TestSnapshotsDoNotPublishPrivateInputs(t *testing.T) {
	for _, name := range []string{":(exclude)*.js", " leading.go", "testdata/key.pem"} {
		t.Run(name, func(t *testing.T) {
			d := repo(t)
			put(t, d, name, "private test input\n")
			put(t, d, "private.txt", "another private input\n")
			before := command(t, d, "git", "status", "--porcelain=v1")
			tree, err := findings.Snapshot(d)
			if err != nil {
				t.Fatal(err)
			}
			blob := command(t, d, "git", "hash-object", name)
			if out, err := exec.Command("git", "-C", d, "cat-file", "-e", blob).CombinedOutput(); err == nil {
				t.Fatalf("private input published: %s", out)
			}
			if before != command(t, d, "git", "status", "--porcelain=v1") {
				t.Fatal("snapshot changed the index")
			}
			if got := snapshotCommand(t, d, "show", tree+":"+name); got != "private test input" {
				t.Fatalf("literal path lost: %q", got)
			}
			other := command(t, d, "git", "hash-object", "private.txt")
			if exec.Command("git", "-C", d, "cat-file", "-e", other).Run() == nil {
				t.Fatal("magic pathspec published another private file")
			}
		})
	}
}

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
	a, code := findings.Next(d, wait)
	if code != 1 || a.Next != "fix" || !strings.Contains(a.Text(), "TestOther") {
		t.Fatalf("unrecorded failure hidden: %d %+v", code, a)
	}
}

func TestARejectedWitnessMovedElsewhereStillBlocks(t *testing.T) {
	d := openRoundOne(t)
	if _, code := findings.SetVerdict(d, "F1", "rejected", "not promised"); code != 0 {
		t.Fatal(code)
	}
	if err := os.Rename(filepath.Join(d, "value_test.go"), filepath.Join(d, "moved_test.go")); err != nil {
		t.Fatal(err)
	}
	a, code := findings.Next(d, wait)
	if code != 1 || a.Next != "fix" {
		t.Fatalf("moved rejection hidden: %d %+v", code, a)
	}
}

func TestRecordCannotSwallowAnEditAfterThePendingAttack(t *testing.T) {
	d := openRoundOne(t)
	put(t, d, "value.go", fixedValue)
	next(t, d, 1)
	put(t, d, "value.go", fixedValue+"func AddedAfterAttack() int {return 3}\n")
	if _, code := findings.Record(d, "", wait); code != 1 {
		t.Fatal(code)
	}
	a, code := findings.Next(d, wait)
	if code != 1 || a.Next != "tsp" || a.Round != 3 {
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

func TestAClosedLoopRechecksAnUntrackedFixture(t *testing.T) {
	d := repo(t)
	put(t, d, "fixtures/input.txt", "ok")
	put(t, d, "fixture_test.go", "package sample\nimport (\"testing\";\"os\")\nfunc TestFixture(t *testing.T){b,e:=os.ReadFile(\"fixtures/input.txt\");if e!=nil||string(b)!=\"ok\" {t.Fatal(\"input\")}}\n")
	if out, code := findings.Record(d, "", wait); code > 1 {
		t.Fatalf("%d: %s", code, out)
	}
	next(t, d, 0)
	put(t, d, "fixtures/input.txt", "bad")
	a, code := findings.Next(d, wait)
	if code != 1 || a.Next != "fix" {
		t.Fatalf("stale done after fixture edit: %d %+v", code, a)
	}
}

func TestAClosedLoopRecordsANewSuiteOnTheSameTree(t *testing.T) {
	d := repo(t)
	put(t, d, "ok.test.js", "const {test}=require('node:test');test('ok',()=>{});\n")
	put(t, d, "bad.test.js", "const {test}=require('node:test');test('bad',()=>{throw Error('bad')});\n")
	if _, code := findings.Record(d, "node --test ok.test.js", wait); code != 1 {
		t.Fatal(code)
	}
	next(t, d, 0)
	if _, code := findings.Record(d, "node --test bad.test.js", wait); code != 0 {
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
	command(t, d, "git", "add", "-A")
	command(t, d, "git", "commit", "-qm", "existing suites")
	for _, suite := range []string{"node --test a.test.js", "node --test b.test.js"} {
		if _, code := findings.Record(d, suite, wait); code != 1 {
			t.Fatal(code)
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
