package main

import (
	"os"
	"path/filepath"
	"testing"
)

// archRepo lays out a module with a declaration and returns its directory, leaving the caller to write
// the change that the probe will blame.
func archRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(name, body string) {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/m\n\ngo 1.24\n")
	write(".tsp-arch.json", `{"layers":[
		{"name":"svc","paths":["svc/"],"imports":["example.com/m/svc"]},
		{"name":"cmd","paths":["cmd/"],"imports":["example.com/m/cmd"]}],
		"forbid":[{"from":"svc","to":"cmd","why":"a service must not reach into the command line"}]}`)
	write("svc/svc.go", "package svc\n\n// Name is a service name.\nconst Name = \"svc\"\n")
	write("cmd/root.go", "package cmd\n\n// Root is a command.\nconst Root = \"root\"\n")
	verifyGit(t, dir, "init", "-q")
	verifyGit(t, dir, "add", "-A")
	verifyGit(t, dir, "-c", "user.name=tsp", "-c", "user.email=tsp@example.com", "commit", "-qm", "init")
	return dir
}

// TestProbeArchExitCodes pins the probe's contract: 1 when the change added a refused edge, 0 when it
// added none, and 2 when nothing could be measured — never 0 for an unmeasurable run.
func TestProbeArchExitCodes(t *testing.T) {
	t.Run("a refused edge is a finding", func(t *testing.T) {
		dir := archRepo(t)
		if err := os.WriteFile(filepath.Join(dir, "svc", "svc.go"), []byte("package svc\n\nimport \"example.com/m/cmd\"\n\nconst Name = \"svc\" + cmd.Root\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if code := runProbeArch([]string{"--cwd", dir}); code != 1 {
			t.Fatalf("exit = %d, want 1 for a forbidden edge", code)
		}
	})

	t.Run("an untouched edge stays quiet", func(t *testing.T) {
		dir := archRepo(t)
		if err := os.WriteFile(filepath.Join(dir, "cmd", "root.go"), []byte("package cmd\n\n// Root is a command.\nconst Root = \"root2\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if code := runProbeArch([]string{"--cwd", dir}); code != 0 {
			t.Fatalf("exit = %d, want 0 when the change added no refused edge", code)
		}
	})

	t.Run("no declaration measures nothing", func(t *testing.T) {
		dir := archRepo(t)
		if err := os.Remove(filepath.Join(dir, ".tsp-arch.json")); err != nil {
			t.Fatal(err)
		}
		if code := runProbeArch([]string{"--cwd", dir}); code != 2 {
			t.Fatalf("exit = %d, want 2 when no rule can be enforced", code)
		}
	})

	t.Run("a stray argument is a usage error", func(t *testing.T) {
		if code := runProbeArch([]string{"--cwd", t.TempDir(), "extra"}); code != 2 {
			t.Fatalf("exit = %d, want 2 for an unexpected argument", code)
		}
	})
}
