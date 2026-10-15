package bench

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// A Go run fills the per-run home with a module cache whose files and directories are read-only, as
// `go mod download` leaves them. Removing the copy with a plain RemoveAll failed on them and the error
// was dropped, so every run left its whole module cache behind on disk.
func TestFinishRemovesAReadOnlyAgentConfig(t *testing.T) {
	dir := t.TempDir()
	ws := filepath.Join(dir, "ws")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	mod := filepath.Join(dir, agentConfigDir, "home", "go", "pkg", "mod", "example.com", "m@v1.0.0")
	if err := os.MkdirAll(mod, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mod, "m.go"), []byte("package m\n"), 0o444); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(dir, agentConfigDir)
	makeReadOnly := func(writable bool) {
		_ = filepath.WalkDir(cfg, func(p string, d fs.DirEntry, err error) error {
			if err == nil && d.IsDir() {
				mode := os.FileMode(0o555)
				if writable {
					mode = 0o755
				}
				_ = os.Chmod(p, mode)
			}
			return nil
		})
	}
	makeReadOnly(false)
	t.Cleanup(func() { makeReadOnly(true) })

	res := finish(Result{Workspace: ws}, Options{Keep: true}, false)

	if _, err := os.Stat(cfg); !os.IsNotExist(err) {
		t.Fatalf("agent-config still on disk after finish (stat err = %v)", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "result.json")); err != nil {
		t.Fatalf("result.json not written: %v", err)
	}
	if _, err := os.Stat(ws); err != nil || res.Workspace != ws {
		t.Fatalf("a kept workspace must stay: stat err = %v, workspace = %q", err, res.Workspace)
	}
}
