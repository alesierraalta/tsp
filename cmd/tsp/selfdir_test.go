package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAgentBinDirPutsATspNamedEntryFirstForARenamedBuild(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "tsp-candidate")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, cleanup, err := agentBinDir(exe)
	if err != nil {
		t.Fatalf("agentBinDir: %v", err)
	}
	target, err := filepath.EvalSymlinks(filepath.Join(got, "tsp"))
	if err != nil {
		t.Fatalf("no tsp entry in %s: %v", got, err)
	}
	want, _ := filepath.EvalSymlinks(exe)
	if target != want {
		t.Fatalf("tsp in %s resolves to %s, want the build under test %s", got, target, want)
	}
	cleanup()
	if _, err := os.Stat(got); !os.IsNotExist(err) {
		t.Fatalf("the staged directory %s outlived its run: %v", got, err)
	}
	if _, err := os.Stat(exe); err != nil {
		t.Fatalf("cleanup removed the build under test: %v", err)
	}
}

func TestAgentBinDirIsTheBinaryDirectoryWhenItIsNamedTsp(t *testing.T) {
	for _, name := range []string{"tsp", "tsp.exe", "TSP.EXE"} {
		dir := t.TempDir()
		exe := filepath.Join(dir, name)
		if err := os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		got, cleanup, err := agentBinDir(exe)
		if err != nil || got != dir {
			t.Fatalf("%s: agentBinDir = %q, %v; want %q", name, got, err, dir)
		}
		cleanup()
		if _, err := os.Stat(exe); err != nil {
			t.Fatalf("%s: cleanup touched the binary's own directory: %v", name, err)
		}
	}
}
