package check

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A caller spells its paths from the directory it asked about, and --cwd keeps its default "." there.
// Reading such a value as root-relative answered "nothing to check" for a file the caller had just
// named, and the verifier read that answer as "this change owes no test".
func TestCheckScopedPathsResolveRelativeToTheDirectoryTheCallerStandsIn(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(filepath.Join(root, "sub"))
	repo := &fakeRepo{root: root, status: porcelain(" M sub/src/app.js")}
	deps := repo.deps()
	deps.Paths = []string{"src/app.js"}
	res := Run(".", deps)
	if res.Exit != 1 {
		t.Fatalf("exit = %d, want the debt of the file named from sub:\n%s", res.Exit, res.Text)
	}
	if !strings.Contains(res.Text, "no test was added or changed with it") {
		t.Fatalf("text missed the debt of sub/src/app.js:\n%s", res.Text)
	}
}

// A relative --cwd is relative to the process, not to the repository root: the two agree only when the
// process stands at the root, which is why this spelling has to keep answering the same way.
func TestCheckScopedPathsResolveARelativeCwdAgainstTheProcess(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	repo := &fakeRepo{root: root, status: porcelain(" M sub/src/app.js")}
	deps := repo.deps()
	deps.Paths = []string{"src/app.js"}
	res := Run("sub", deps)
	if res.Exit != 1 {
		t.Fatalf("exit = %d, want the debt of the file named from sub:\n%s", res.Exit, res.Text)
	}
	if !strings.Contains(res.Text, "no test was added or changed with it") {
		t.Fatalf("text missed the debt of sub/src/app.js:\n%s", res.Text)
	}
}
