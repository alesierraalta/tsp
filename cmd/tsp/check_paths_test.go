package main

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// pathsRepo is a real git repository with the named files committed, so `tsp check --paths` answers
// from a status git itself produced.
func pathsRepo(t *testing.T, files ...string) string {
	t.Helper()
	dir := t.TempDir()
	gitIn := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	gitIn("init", "-q")
	for _, f := range files {
		write := filepath.Join(dir, f)
		if err := os.MkdirAll(filepath.Dir(write), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(write, []byte("package sample\n\nfunc F() {}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitIn("add", ".")
	gitIn("commit", "-qm", "base")
	return dir
}

func writeIn(t *testing.T, dir, file, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, file), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// runCheckCapturing runs the check subcommand with both streams captured.
func runCheckCapturing(t *testing.T, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	rOut, wOut, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	rErr, wErr, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origOut, origErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = wOut, wErr
	code = runCheck(args)
	wOut.Close()
	wErr.Close()
	os.Stdout, os.Stderr = origOut, origErr
	out, _ := io.ReadAll(rOut)
	errOut, _ := io.ReadAll(rErr)
	rOut.Close()
	rErr.Close()
	return code, string(out), string(errOut)
}

// A changed file whose name carries a comma is one path: --paths takes it whole, and the check
// answers for that file. Split into "a" and "b" it would name nothing and answer "nothing to
// check" while the unscoped reading reports the debt.
func TestCheckPathsTakesACommaInTheNameAsOnePath(t *testing.T) {
	dir := pathsRepo(t, "a,b.go")
	writeIn(t, dir, "a,b.go", "package sample\n\nfunc F() int { return 1 }\n")
	code, out, errOut := runCheckCapturing(t, "--cwd", dir, "--paths", "a,b.go")
	if code != 1 || !strings.Contains(out, "no test was added or changed with it") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	if strings.Contains(out, "nothing to check") {
		t.Fatalf("the comma-carrying path must not be read as a list of names that changed nothing:\n%s", out)
	}
}

// Every --paths occurrence names one path, and every one is answered for.
func TestCheckPathsHonorsEachOccurrence(t *testing.T) {
	dir := pathsRepo(t, "a.go", "b.go")
	writeIn(t, dir, "a.go", "package sample\n\nfunc A() int { return 1 }\n")
	writeIn(t, dir, "b.go", "package sample\n\nfunc B() int { return 1 }\n")
	code, out, errOut := runCheckCapturing(t, "--cwd", dir, "--paths", "a.go", "--paths", "b.go")
	if code != 1 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	if !strings.Contains(out, "2 production source file(s) changed: a.go, b.go") {
		t.Fatalf("both named paths must reach the answer:\n%s", out)
	}
}

// The legacy comma-joined value names a file that does not exist. It must fail loudly by name
// instead of answering "nothing to check" for a path the caller never passed.
func TestCheckPathsRefusesACommaJoinedList(t *testing.T) {
	dir := pathsRepo(t, "a.go", "b.go")
	writeIn(t, dir, "a.go", "package sample\n\nfunc A() int { return 1 }\n")
	code, out, errOut := runCheckCapturing(t, "--cwd", dir, "--paths", "a.go,b.go")
	if code != 2 {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	for _, want := range []string{"--paths", "a.go,b.go", "neither a file nor a changed path"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q: %q", want, out)
		}
	}
}

// A deleted tracked file is still part of the change: git status shows it, so naming it in --paths
// is not a mistake, however much is left on disk to stat.
func TestCheckPathsAcceptsADeletedTrackedFile(t *testing.T) {
	dir := pathsRepo(t, "a.go", "b.go")
	if err := os.Remove(filepath.Join(dir, "a.go")); err != nil {
		t.Fatal(err)
	}
	writeIn(t, dir, "b.go", "package sample\n\nfunc B() int { return 1 }\n")
	code, out, errOut := runCheckCapturing(t, "--cwd", dir, "--paths", "a.go", "--paths", "b.go")
	if code != 1 {
		t.Fatalf("b.go came with no test, so the scoped answer still owes one: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	if strings.Contains(out+errOut, "neither a file nor a changed path") {
		t.Fatalf("a deleted tracked file is a changed path and must not be refused:\n%s\n%s", out, errOut)
	}
	// The deletion alone owes no test, and the guard must not turn that quiet answer into a refusal.
	quiet, out, errOut := runCheckCapturing(t, "--cwd", dir, "--paths", "a.go")
	if quiet != 0 || !strings.Contains(out, "no production source in the named paths is changed") {
		t.Fatalf("code=%d stdout=%q stderr=%q", quiet, out, errOut)
	}
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "user.email=t@t", "-c", "user.name=t"}, args...)...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// A staged rename is one change with two names: git status prints `R  old -> new`. Naming the old
// name, alone or beside the new one, names a changed path, so it must never turn the answer into a
// refusal that hides the debt the new name owes.
func TestCheckPathsAcceptsBothNamesOfAStagedRename(t *testing.T) {
	dir := pathsRepo(t, "a.go", "b.go")
	gitRun(t, dir, "mv", "a.go", "a2.go")
	code, out, errOut := runCheckCapturing(t, "--cwd", dir, "--paths", "a.go", "--paths", "a2.go")
	if strings.Contains(out+errOut, "neither a file nor a changed path") {
		t.Fatalf("the old name of a staged rename is a changed path and must not be refused:\n%s\n%s", out, errOut)
	}
	if code != 1 || !strings.Contains(out, "a2.go") {
		t.Fatalf("the renamed file came with no test, so the answer still owes one: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}

// A tracked symlink whose target is gone is still a name in the repository. Asking whether the
// name exists must not follow the link and report the missing target as a missing name.
func TestCheckPathsKeepsATrackedSymlinkWithAMissingTarget(t *testing.T) {
	dir := pathsRepo(t, "a.go", "b.go")
	if err := os.Symlink("gone.go", filepath.Join(dir, "link.go")); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", "link.go")
	gitRun(t, dir, "commit", "-qm", "link")
	writeIn(t, dir, "b.go", "package sample\n\nfunc B() int { return 2 }\n")
	code, out, errOut := runCheckCapturing(t, "--cwd", dir, "--paths", "link.go", "--paths", "b.go")
	if strings.Contains(out+errOut, "neither a file nor a changed path") {
		t.Fatalf("a tracked symlink is a name in the repository, whatever it points at:\n%s\n%s", out, errOut)
	}
	if code != 1 || !strings.Contains(out, "b.go") {
		t.Fatalf("b.go came with no test, so the answer still owes one: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
}
