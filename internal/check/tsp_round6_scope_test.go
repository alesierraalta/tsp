package check

import (
	"strings"
	"testing"
)

// A scoped question has to be answered in a repository without a plan too. test-strategy 0.7 writes
// none, so the repositories this criterion was written for are exactly the ones where the plan is
// absent; falling back to the whole tree there answers a question nobody asked.
func TestCheckScopedPathsAnswerWithoutAPlan(t *testing.T) {
	repo := &fakeRepo{root: "/r", status: porcelain(" M src/app.js"), planErr: true}
	deps := repo.deps()
	deps.Paths = []string{"src/other.js"}
	res := Run(".", deps)
	if res.Exit != 0 {
		t.Fatalf("a scoped question about a path that did not change owes nothing: exit %d\n%s", res.Exit, res.Text)
	}
	if !strings.Contains(res.Text, "no production source in the named paths is changed") {
		t.Fatalf("the answer must speak about the named paths, not the whole tree:\n%s", res.Text)
	}
	if repo.judged != 0 {
		t.Fatalf("a scoped question must not judge the whole tree's red tests: judged %d time(s)", repo.judged)
	}
}

// The named paths are spelled from the directory the caller asked about, and that directory need not
// be the root: the offer passes the paths the run edited, relative to the session's own cwd.
func TestCheckScopedPathsResolveAgainstTheDirectoryAskedAbout(t *testing.T) {
	repo := &fakeRepo{root: "/r", status: porcelain(" M sub/src/app.js"), plan: owing}
	deps := repo.deps()
	deps.Paths = []string{"src/app.js"}
	res := Run("/r/sub", deps)
	if strings.Contains(res.Text, "no production source in the named paths is changed") {
		t.Fatalf("the source named from a subdirectory is changed and must be answered for:\n%s", res.Text)
	}
	if !strings.Contains(res.Text, "sub/src/app.js") {
		t.Fatalf("the answer must name the changed file the caller meant:\n%s", res.Text)
	}
	if res.Exit != 1 {
		t.Fatalf("that source came with no test, so it owes one: exit %d\n%s", res.Exit, res.Text)
	}
}

// A path outside the repository cannot be answered for. Answering that it "changed nothing" reads as
// "this change owes nothing", which is the false negative the scoped question exists to prevent.
func TestCheckScopedPathsRefuseAPathOutsideTheRepository(t *testing.T) {
	for _, named := range []string{"../outside/app.js", "/outside/app.js"} {
		t.Run(named, func(t *testing.T) {
			repo := &fakeRepo{root: "/r", status: porcelain(" M src/app.js"), plan: owing}
			deps := repo.deps()
			deps.Paths = []string{named}
			res := Run("/r", deps)
			if res.Exit == 0 {
				t.Fatalf("a path outside the repository must not be answered as owes-nothing: %s", res.Text)
			}
			if !strings.Contains(res.Text, named) {
				t.Fatalf("the refusal must name the path it cannot answer for:\n%s", res.Text)
			}
		})
	}
}
