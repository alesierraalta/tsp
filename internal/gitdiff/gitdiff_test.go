package gitdiff

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func fake(out string, err error) Runner {
	return func(dir string, args ...string) (string, error) { return out, err }
}

// A commit range is asked for with the three-dot form, so paths are the change's, not the base's drift.
func TestChangedReadsTheRangeWhenBaseIsNamed(t *testing.T) {
	var asked []string
	git := func(dir string, args ...string) (string, error) {
		asked = args
		return "a.go\n b.go \n\n", nil
	}
	paths, err := Changed(".", "origin/main", git)
	if err != nil {
		t.Fatalf("changed: %v", err)
	}
	if len(paths) != 2 || paths[0] != "a.go" || paths[1] != "b.go" {
		t.Fatalf("paths = %v, want the two names without blanks", paths)
	}
	if asked[0] != "diff" || asked[len(asked)-1] != "origin/main...HEAD" {
		t.Fatalf("git args = %v, want a three-dot diff", asked)
	}
}

// The working tree is the default scope, untracked files included: a new file is part of the change.
func TestChangedReadsTheWorkingTreeByDefault(t *testing.T) {
	git := func(dir string, args ...string) (string, error) {
		return " M a.go\x00?? b.go\x00", nil
	}
	paths, err := Changed(".", "", git)
	if err != nil {
		t.Fatalf("changed: %v", err)
	}
	if len(paths) != 2 || paths[0] != "a.go" || paths[1] != "b.go" {
		t.Fatalf("paths = %v, want porcelain entries reduced to names", paths)
	}
}

// A directory git does not know has no change to scope, and says so instead of returning nothing.
func TestRootOutsideARepositoryIsNotARepository(t *testing.T) {
	if _, err := Root("/tmp", fake("", errors.New("fatal"))); !errors.Is(err, ErrNotRepository) {
		t.Fatalf("err = %v, want ErrNotRepository", err)
	}
}

// A caller that names a repository root gets that root back, even through a symlink.
func TestRootWithinAcceptsTheNamedDirectory(t *testing.T) {
	dir := t.TempDir()
	alias := filepath.Join(filepath.Dir(dir), filepath.Base(dir)+"-link")
	if err := os.Symlink(dir, alias); err != nil {
		t.Skipf("cannot make a symlink here: %v", err)
	}
	root, err := RootWithin(alias, fake(dir+"\n", nil))
	if err != nil || root != dir {
		t.Fatalf("root = %q, err = %v, want %q", root, err, dir)
	}
}

// The directory the caller gave is the boundary: an ancestor repository is not its repository.
func TestRootWithinRefusesAnAncestor(t *testing.T) {
	top := t.TempDir()
	inner := filepath.Join(top, "nested")
	if err := os.Mkdir(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	root, err := RootWithin(inner, fake(top+"\n", nil))
	if !errors.Is(err, ErrRootOutsideDir) {
		t.Fatalf("err = %v, want ErrRootOutsideDir", err)
	}
	if root != top {
		t.Fatalf("root = %q, want the resolved ancestor %q for the error message", root, top)
	}
}
