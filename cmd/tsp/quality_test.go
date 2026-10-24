package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestProbeQualityReportsOnARealChange drives the probe over a real repository: a complex function and a
// copied block inside the change exit 1, and a change the probe cannot measure exits 2 instead of passing.
func TestProbeQualityReportsOnARealChange(t *testing.T) {
	dir := t.TempDir()
	verifyGit(t, dir, "init", "-q")
	verifyGit(t, dir, "config", "user.email", "test@example.com")
	verifyGit(t, dir, "config", "user.name", "test")
	base := "package app\n\nfunc Add(a, b int) int { return a + b }\n"
	if err := os.WriteFile(filepath.Join(dir, "app.go"), []byte(base), 0o644); err != nil {
		t.Fatal(err)
	}
	verifyGit(t, dir, "add", ".")
	verifyGit(t, dir, "commit", "-qm", "base")

	block := "\tvalue := compute(input)\n\tstore(value)\n\ttotal += len(value)\n\tif total > limit {\n\t\treset(input)\n\t}\n"
	change := "package app\n\nfunc Add(a, b int) int { return a + b }\n\nfunc Sprawl(n int) int {\n\ttotal := 0\n\tfor i := 0; i < n; i++ {\n\t\tif i%2 == 0 {\n\t\t\tif i%3 == 0 {\n\t\t\t\tswitch i {\n\t\t\t\tcase 0:\n\t\t\t\t\ttotal++\n\t\t\t\tcase 1:\n\t\t\t\t\ttotal--\n\t\t\t\t}\n\t\t\t}\n\t\t}\n\t}\n\treturn total\n}\n\nfunc Copy() {\n" + block + block + "}\n"
	if err := os.WriteFile(filepath.Join(dir, "app.go"), []byte(change), 0o644); err != nil {
		t.Fatal(err)
	}

	if code := runProbeQuality([]string{"--cwd", dir, "--base", "HEAD"}); code != 1 {
		t.Fatalf("exit = %d, want 1 for a change with a complex function and a copied block", code)
	}

	// Only Python changed: no adapter, so the probe must refuse to report clean.
	if err := os.WriteFile(filepath.Join(dir, "tool.py"), []byte("def f():\n    return 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	verifyGit(t, dir, "checkout", "--", "app.go")
	if code := runProbeQuality([]string{"--cwd", dir}); code != 2 {
		t.Fatalf("exit = %d, want 2 when the only changed source is a language without an adapter", code)
	}
}
