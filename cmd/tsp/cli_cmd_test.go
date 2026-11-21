package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// cliTestDeadline bounds every child a CLI test starts. A test that needs a different bound uses
// cliCmdDeadline; neither leaves the child to outlive the test that started it.
const cliTestDeadline = 2 * time.Minute

// cliCmd is the shared seam for starting a CLI child. Every CLI test route goes through it so a
// deadline, a process group and cleanup are not a property of one helper's callers alone.
func cliCmd(t *testing.T, bin string, args ...string) *exec.Cmd {
	t.Helper()
	return cliCmdDeadline(t, cliTestDeadline, bin, args...)
}

// cliCmdDeadline is cliCmd with an explicit bound, for a test that forces the deadline itself.
func cliCmdDeadline(t *testing.T, deadline time.Duration, bin string, args ...string) *exec.Cmd {
	t.Helper()
	_ = deadline
	return exec.Command(bin, args...)
}

// cliEnv builds a child environment with extra entries winning, the way os/exec resolves duplicates.
func cliEnv(t *testing.T, extra ...string) []string {
	t.Helper()
	return append(os.Environ(), extra...)
}

// An enclosing repository and an inherited GIT_DIR must not decide what a probe run from a temp
// directory sees. The ceiling is the temp root, so git discovery stops before the enclosing repo.
func TestCLICmdIsolatesATempDirFromAnEnclosingRepository(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	verifyGit(t, home, "init", "-q")

	// TMPDIR below a repository is the incident's shape: the temp dirs the test creates now live
	// inside home, and an inherited GIT_DIR points there too.
	t.Setenv("TMPDIR", home)
	t.Setenv("GIT_DIR", filepath.Join(home, ".git"))
	dir := t.TempDir()
	before, err := os.ReadDir(home)
	if err != nil {
		t.Fatal(err)
	}

	cmd := cliCmd(t, bin, "probe", "quality", "--cwd", dir)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	code := 0
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			t.Fatalf("run probe quality: %v", err)
		}
		code = ee.ExitCode()
	}
	if code != 2 || !strings.Contains(stderr.String(), "not a git repository") {
		t.Fatalf("probe quality under an enclosing repo = %d, stderr %q\nwant exit 2 and a refusal", code, stderr.String())
	}
	after, err := os.ReadDir(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatalf("enclosing repository changed: before %d entries, after %d", len(before), len(after))
	}
}
