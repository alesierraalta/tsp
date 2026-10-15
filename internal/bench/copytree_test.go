package bench

import (
	"os"
	"path/filepath"
	"testing"
)

// A fixture's symlinks reach the workspace as symlinks: suites that test link handling (gitleaks' symlinked
// keys) fail before the agent starts when the copy drops them. A fix overlay that carries the same link copies
// over it instead of failing on the existing one.
func TestCopyTreeKeepsSymlinksAndOverlaysThem(t *testing.T) {
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "dir", "key"), []byte("k"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("key", filepath.Join(src, "dir", "link")); err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()
	for i := 0; i < 2; i++ {
		if err := copyTree(src, dst); err != nil {
			t.Fatalf("copy #%d: %v", i+1, err)
		}
	}
	target, err := os.Readlink(filepath.Join(dst, "dir", "link"))
	if err != nil {
		t.Fatalf("the symlink was not copied: %v", err)
	}
	if target != "key" {
		t.Fatalf("link target = %q, want %q", target, "key")
	}
	if b, err := os.ReadFile(filepath.Join(dst, "dir", "link")); err != nil || string(b) != "k" {
		t.Fatalf("reading through the link: %q, %v", b, err)
	}
}

// A link that resolves outside the fixture is not copied at all, so a fixture still cannot reach the host.
func TestCopyTreeSkipsLinksThatLeaveTheFixture(t *testing.T) {
	src := t.TempDir()
	for _, l := range []struct{ name, target string }{{"abs", "/etc/hostname"}, {"up", "../outside"}} {
		if err := os.Symlink(l.target, filepath.Join(src, l.name)); err != nil {
			t.Fatal(err)
		}
	}
	dst := t.TempDir()
	if err := copyTree(src, dst); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"abs", "up"} {
		if _, err := os.Lstat(filepath.Join(dst, name)); !os.IsNotExist(err) {
			t.Fatalf("link %s leaving the fixture was copied: %v", name, err)
		}
	}
}
