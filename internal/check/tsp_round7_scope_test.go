package check

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A caller may stand in a directory it reached through a symlink. git resolves that directory to its
// real place inside the repository, so every path the check derives from git — the root, and the
// changed files it compares against — is spelled from the real location. A --cwd that keeps the
// link's own spelling, and the named paths prefixed with it, therefore name a file git never
// reported as changed, and the caller is told "nothing to check" for the one source it just named:
// the answer the verifier reads as "this change owes no test", for a file that does owe one.
func TestCheckScopedPathsResolveACwdReachedThroughASymlink(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link_to_sub")
	if err := os.Symlink(filepath.Join(root, "sub"), link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	repo := &fakeRepo{root: root, status: porcelain(" M sub/src_app.js")}
	deps := repo.deps()
	deps.Paths = []string{"src_app.js"}
	res := Run(link, deps)
	if res.Exit != 1 {
		t.Fatalf("exit = %d, want the debt of the file named from the directory the link names (sub/src_app.js):\n%s", res.Exit, res.Text)
	}
	if !strings.Contains(res.Text, "sub/src_app.js") {
		t.Fatalf("text missed sub/src_app.js, the changed file the caller named through the link:\n%s", res.Text)
	}
}

// --cwd is documented as "directory inside the repository to check", and the scoped reading refuses a
// path outside the repository "rather than answered, because 'it did not change' would read as 'this
// change owes nothing'". A cwd that is not a directory inside the repository — outside it, absent, or
// a file — leaves no prefix to scope the named paths with, so they are read as root-relative: the
// caller is answered for a file it never named, either accused of the root's debt or told the tree is
// clean. Both are answers to a question the check could not have read; it owes a refusal instead.
func TestCheckScopedPathsRefuseACwdThatIsNotADirectoryInsideTheRepository(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(sub, "src_app.js")
	if err := os.WriteFile(file, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		cwd  string
	}{
		{"outside the repository", t.TempDir()},
		{"a directory that does not exist", filepath.Join(root, "sub", "missing")},
		{"a file", file},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{root: root, status: porcelain(" M sub/src_app.js")}
			deps := repo.deps()
			deps.Paths = []string{"sub/src_app.js"}
			res := Run(tc.cwd, deps)
			if res.Exit != 2 {
				t.Fatalf("cwd=%s exit = %d, want the refusal a cwd that is not a directory inside the repository earns:\n%s", tc.cwd, res.Exit, res.Text)
			}
			if strings.Contains(res.Text, "nothing to check") {
				t.Fatalf("cwd=%s was answered as a clean tree instead of refused:\n%s", tc.cwd, res.Text)
			}
		})
	}
}
