package bench

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// enclosingRepo makes a repository whose worktree contains dir: the machine condition in #481, where
// a benchmark's temp directories sit under the operator's home and that home is itself a repository.
func enclosingRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q"}, {"config", "user.email", "t@example.com"}, {"config", "user.name", "t"},
		{"add", "-A"}, {"commit", "-qm", "base", "--allow-empty"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return root
}

// A suite that asserts "outside a repository" must behave the same here as on a machine whose temp
// root is outside any repository. Without a ceiling git climbs from the workspace into the enclosing
// repository and the suite is wrongly green.
func TestSuiteUnderAnEnclosingRepositoryCannotSeeIt(t *testing.T) {
	root := enclosingRepo(t)
	dir := filepath.Join(root, "runs", "case", "ws")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	res, err := RunSuite(dir, "git rev-parse --is-inside-work-tree", 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if res.Green || !strings.Contains(res.Output, "not a git repository") {
		t.Fatalf("suite = %+v, want a non-git directory even though %s is a repository", res, root)
	}
}

// The suite's own temp dirs are under TMPDIR, which may be somewhere else entirely: a ceiling on the
// workspace alone would still let git discover the repository above TMPDIR (as in o17's TestD6ExitCodes).
func TestSuiteTempDirUnderAnEnclosingRepositoryCannotSeeIt(t *testing.T) {
	root := enclosingRepo(t)
	tmp := filepath.Join(root, "bench-runs", "tmp")
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", tmp)
	res, err := RunSuite(tmp, `sh -c 'cd "$TMPDIR" && git rev-parse --is-inside-work-tree'`, 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if res.Green || !strings.Contains(res.Output, "not a git repository") {
		t.Fatalf("suite = %+v, want TMPDIR to be outside any visible repository", res)
	}
}

// Catch checks run their suites through runCapture rather than RunSuite: they need the same ceiling.
func TestCatchCheckUnderAnEnclosingRepositoryCannotSeeIt(t *testing.T) {
	root := enclosingRepo(t)
	dir := filepath.Join(root, "stage", "case")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	code, out := runCapture(dir, []string{"git", "rev-parse", "--is-inside-work-tree"}, 30*time.Second)
	if code == 0 || !strings.Contains(out, "not a git repository") {
		t.Fatalf("runCapture = %d %q, want a non-git directory", code, out)
	}
}

func TestBenchProcessEnvCeilsTheWorkspaceAndItsTempDir(t *testing.T) {
	t.Setenv("TMPDIR", "/home/ale/bench-runs/tmp")
	env := benchProcessEnv("/home/ale/bench-runs/case/ws", []string{
		"PATH=/bin", "GIT_CEILING_DIRECTORIES=/inherited",
	})
	want := []string{"/home/ale/bench-runs/case" + string(os.PathListSeparator) + "/home/ale/bench-runs/tmp"}
	if got := envValues(env, "GIT_CEILING_DIRECTORIES"); !reflect.DeepEqual(got, want) {
		t.Fatalf("ceiling = %q, want %q (inherited value replaced)", got, want)
	}
}

// Every runner builds its process env from the same base; none of the wrapper helpers may drop it.
func TestAgentEnvironmentsKeepTheGitCeiling(t *testing.T) {
	t.Setenv("TMPDIR", "/home/ale/bench-runs/tmp")
	base := benchProcessEnv("/home/ale/bench-runs/case/ws", []string{"PATH=/bin"})
	want := envValues(base, "GIT_CEILING_DIRECTORIES")
	builders := map[string]func([]string) []string{
		"claude": func(e []string) []string { return agentEnv(e, "/cfg", "/bin") },
		"pi":     func(e []string) []string { return piEnv(e, "/cfg", "/bin") },
		"pi product": func(e []string) []string {
			return piProductEnv(e, "/cfg", "/bin")
		},
		"codex": func(e []string) []string { return codexEnv(e, "/cfg", "/bin") },
		"agy":   func(e []string) []string { return agyEnv(e, "/cfg", "/bin") },
	}
	for name, build := range builders {
		t.Run(name, func(t *testing.T) {
			if got := envValues(build(base), "GIT_CEILING_DIRECTORIES"); !reflect.DeepEqual(got, want) {
				t.Fatalf("ceiling = %q, want %q", got, want)
			}
		})
	}
}
