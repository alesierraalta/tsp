package check

import (
	"strings"
	"testing"
)

// In CI the candidate is a commit range: the working tree holds HEAD, so a check that reads git
// status finds nothing changed and passes every pull request.
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

// A waiver lives in the working tree; a commit range has none, so the two cannot be asked together.
func TestBaseRefusesTheWorkingTreeFlags(t *testing.T) {
	cases := []struct {
		name string
		set  func(*Deps)
	}{
		{"waiver", func(d *Deps) { d.Waive = "renamed a local" }},
		{"paths", func(d *Deps) { d.Paths = []string{"src/app.go"} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := (&fakeRepo{root: "/repo"}).deps()
			d.Base = "origin/main"
			tc.set(&d)
			res := Run(".", d)
			if res.Exit != 2 || !strings.Contains(res.Text, "--base") {
				t.Fatalf("verdict = %q (exit %d), want a refusal naming --base", res.Text, res.Exit)
			}
		})
	}
}
