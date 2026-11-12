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
	got, err := agentBinDir(exe)
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
}

func TestAgentBinDirIsTheBinaryDirectoryWhenItIsNamedTsp(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "tsp")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := agentBinDir(exe)
	if err != nil || got != dir {
		t.Fatalf("agentBinDir = %q, %v; want %q", got, err, dir)
	}
}
