package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mutateEngine writes a stand-in for gremlins that answers the two passes the harness makes: the
// counting pass (-d) and the real one. The harness never trusts the engine's prose, only this file.
func mutateEngine(t *testing.T, status string) {
	t.Helper()
	dir := t.TempDir()
	body := `#!/bin/sh
out=""
mutation='{"files":[{"file_name":"main.go","mutations":[{"type":"CONDITIONALS_NEGATION","status":"STATUS","line":3}]}],"mutants_total":1}'
while [ $# -gt 0 ]; do
  case "$1" in
    -o) out="$2"; shift 2 ;;
    *) shift ;;
  esac
done
printf '%s' "$mutation" > "$out"
`
	script := filepath.Join(dir, "gremlins")
	if err := os.WriteFile(script, []byte(strings.ReplaceAll(body, "STATUS", status)), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// A change that touched no Go source owes no mutation run: no engine, no cost, no complaint.
func TestMutateWithoutGoSourceExitsZero(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("docs\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"config", "user.name", "t"}, {"config", "user.email", "t@e.c"}, {"add", "-A"}, {"commit", "-q", "-m", "base"}} {
		verifyGit(t, dir, args...)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("docs changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", "/usr/bin:/bin")
	if code := runMutate([]string{"--cwd", dir}); code != 0 {
		t.Fatalf("exit = %d, want 0 for a change with no Go source", code)
	}
}

// Without an engine nothing was measured, and a run that measured nothing must not read as a pass.
func TestMutateWithoutEngineExitsTwo(t *testing.T) {
	dir := verifyTestRepo(t)
	t.Setenv("PATH", "/usr/bin:/bin")
	if code := runMutate([]string{"--cwd", dir}); code != 2 {
		t.Fatalf("exit = %d, want 2 when the engine is missing", code)
	}
}

// A surviving mutant is the finding: the change is not pinned, and the gate says so.
func TestMutateWithSurvivorExitsOne(t *testing.T) {
	dir := verifyTestRepo(t)
	mutateEngine(t, "LIVED")
	if code := runMutate([]string{"--cwd", dir, "--fail-on-survivors", "0"}); code != 1 {
		t.Fatalf("exit = %d, want 1 when a mutant survives", code)
	}
	// The same run with a report-only threshold publishes the survivor and gates nothing.
	if code := runMutate([]string{"--cwd", dir, "--fail-on-survivors", "-1"}); code != 0 {
		t.Fatalf("exit = %d, want 0 when the threshold is report-only", code)
	}
}
