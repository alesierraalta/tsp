package check

import (
	"strings"
	"testing"
)

// In CI the candidate is a commit range: the working tree holds HEAD, so a check that reads git
// status finds nothing changed and passes every pull request.
//
// The fake repository here answers the range questions, so this test pins what the verdict says, not
// that the base reaches the red check. That wiring is only exercised end to end: `tsp check --base` in
// .github/workflows/testing-gate.yml is what runs it against a real base on every pull request, and a
// regression there shows up as a gate that stops refusing untested source.
func TestBaseChecksTheCommitRangeNotTheWorkingTree(t *testing.T) {
	repo := &fakeRepo{root: "/repo", diffOut: "src/app.go\x00"}
	d := repo.deps()
	d.Base = "origin/main"
	res := Run(".", d)
	if res.Exit != 1 {
		t.Fatalf("want the range's untested source to fail, got exit %d: %s", res.Exit, res.Text)
	}
	if !strings.Contains(res.Text, "src/app.go") || !strings.Contains(res.Text, "origin/main") {
		t.Fatalf("verdict = %q, want the changed file and the range that carries it", res.Text)
	}
}

func TestBaseLeavesAnUntouchedTreeQuiet(t *testing.T) {
	repo := &fakeRepo{root: "/repo", status: " M src/app.go\n"}
	d := repo.deps()
	d.Base = "origin/main"
	res := Run(".", d)
	if res.Exit != 0 || !strings.Contains(res.Text, "no production source") {
		t.Fatalf("verdict = %q (exit %d), want a quiet verdict for a range that changed no source", res.Text, res.Exit)
	}
}

// A path list answers for the working tree a person is looking at, and a commit range has none, so the
// two cannot be asked together. A waiver can, and is covered by TestBaseModeHonoursAWaiverCommittedWithTheRange.
func TestBaseRefusesTheWorkingTreeFlags(t *testing.T) {
	d := (&fakeRepo{root: "/repo"}).deps()
	d.Base = "origin/main"
	d.Paths = []string{"src/app.go"}
	res := Run(".", d)
	if res.Exit != 2 || !strings.Contains(res.Text, "--base") {
		t.Fatalf("verdict = %q (exit %d), want a refusal naming --base", res.Text, res.Exit)
	}
}
