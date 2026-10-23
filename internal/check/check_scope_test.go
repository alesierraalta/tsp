package check

import (
	"strings"
	"testing"
)

// The plan's remaining targets are the run's agenda, not this change's debt. Scoped to the paths the
// work touched, check answers one question — do these production source files owe a test? — and the
// answer never depends on how much of the plan is still open.
func TestCheckScopedPathsIgnoreThePlanTargets(t *testing.T) {
	cases := []struct {
		name     string
		status   string
		existing []string
		paths    []string
		wantExit int
		wantOut  string
		wantNot  string
	}{
		{
			name:     "source changed without a test owes one",
			status:   porcelain(" M src/app.js"),
			paths:    []string{"src/app.js"},
			wantExit: 1, wantOut: "no test was added or changed with it", wantNot: "layers swept",
		},
		{
			name:     "source changed with a test owes nothing",
			status:   porcelain(" M src/app.js", " M tests/app.test.js"),
			paths:    []string{"src/app.js"},
			wantExit: 0, wantOut: "a test was added or changed with it: tests/app.test.js", wantNot: "layers swept",
		},
		{
			name:     "a path outside the production source owes nothing",
			status:   porcelain(" M src/app.js", " M docs/report.md"),
			paths:    []string{"docs/report.md"},
			wantExit: 0, wantOut: "no production source in the named paths is changed",
		},
		{
			name:     "a named path that did not change owes nothing",
			status:   porcelain(" M src/app.js"),
			existing: []string{"src/other.js"},
			paths:    []string{"src/other.js"},
			wantExit: 0, wantOut: "no production source in the named paths is changed",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{root: "/r", status: tc.status, plan: owing, existing: tc.existing}
			unscoped := Run(".", repo.deps())
			if unscoped.Exit != 1 || !strings.Contains(unscoped.Text, "layers swept") {
				t.Fatalf("the plan should owe a target for this test to mean anything: exit %d\n%s", unscoped.Exit, unscoped.Text)
			}
			deps := repo.deps()
			deps.Paths = tc.paths
			res := Run(".", deps)
			if res.Exit != tc.wantExit {
				t.Fatalf("exit = %d, want %d: %s", res.Exit, tc.wantExit, res.Text)
			}
			if !strings.Contains(res.Text, tc.wantOut) {
				t.Fatalf("text missing %q:\n%s", tc.wantOut, res.Text)
			}
			if tc.wantNot != "" && strings.Contains(res.Text, tc.wantNot) {
				t.Fatalf("the scoped answer must not carry %q:\n%s", tc.wantNot, res.Text)
			}
			if repo.judged != 0 {
				t.Fatalf("the scoped answer re-ran the changed tests %d time(s): it reads the tree, it does not run it", repo.judged)
			}
		})
	}
}

// The tests that settle a scoped change are named like the changed source: a host reads this answer
// through a bounded pipe, so a change carrying hundreds of tests must not print every one of them.
func TestCheckScopedPathsCapTheTestsNamed(t *testing.T) {
	entries := []string{" M src/app.js"}
	for i := 1; i <= MaxNamed+2; i++ {
		entries = append(entries, "?? tests/t"+string(rune('0'+i))+".test.js")
	}
	repo := &fakeRepo{root: "/r", status: porcelain(entries...), plan: owing}
	deps := repo.deps()
	deps.Paths = []string{"src/app.js"}
	res := Run(".", deps)
	want := "a test was added or changed with it: tests/t1.test.js, tests/t2.test.js, tests/t3.test.js, tests/t4.test.js, tests/t5.test.js, tests/t6.test.js and 2 more"
	if res.Exit != 0 || !strings.Contains(res.Text, want) {
		t.Fatalf("exit %d, want 0 with %q:\n%s", res.Exit, want, res.Text)
	}
}

// A --paths name that is neither a file in the working tree nor a path git status shows is a
// mistake, and answering "nothing to check" for it would hide the very debt the caller asked
// about: the check refuses it by name instead. A legacy comma-joined value reaches this guard and
// fails loudly, because "a.go,b.go" is neither a file nor a changed path.
func TestCheckScopedPathsRefuseANameThatNeitherExistsNorChanged(t *testing.T) {
	for _, named := range []string{"src/typo.js", "a.go,b.go"} {
		t.Run(named, func(t *testing.T) {
			repo := &fakeRepo{root: "/r", status: porcelain(" M src/app.js"), plan: owing}
			deps := repo.deps()
			deps.Paths = []string{named}
			res := Run(".", deps)
			if res.Exit != 2 {
				t.Fatalf("exit = %d, want 2: %s", res.Exit, res.Text)
			}
			for _, want := range []string{"--paths", named, "neither a file nor a changed path"} {
				if !strings.Contains(res.Text, want) {
					t.Fatalf("text missing %q:\n%s", want, res.Text)
				}
			}
		})
	}
}

// An existing file that did not change is a legitimate scope that owes nothing, not a refusal: the
// guard exists to catch names that spell nothing, not quiet scopes.
func TestCheckScopedPathsKeepANameForAnExistingFileUnchanged(t *testing.T) {
	repo := &fakeRepo{root: "/r", status: porcelain(" M src/app.js"), plan: owing, existing: []string{"src/other.js"}}
	deps := repo.deps()
	deps.Paths = []string{"src/other.js"}
	res := Run(".", deps)
	if res.Exit != 0 || !strings.Contains(res.Text, "no production source in the named paths is changed") {
		t.Fatalf("exit = %d, want 0 for an existing unchanged file: %s", res.Exit, res.Text)
	}
}

// The opt-out and the waiver keep their meaning inside the scope: a repository that opted out of the
// testing check owes nothing, and neither does a change a person recorded as needing no test.
func TestCheckScopedPathsRespectTheRepositoryOptOutAndTheWaiver(t *testing.T) {
	repo := &fakeRepo{root: "/r", status: porcelain(" M src/app.js"), plan: owing, optOut: true}
	deps := repo.deps()
	deps.Paths = []string{"src/app.js"}
	res := Run(".", deps)
	if res.Exit != 0 || !strings.Contains(res.Text, optOutName) {
		t.Fatalf("exit = %d, want 0 naming the opt-out: %s", res.Exit, res.Text)
	}
}
