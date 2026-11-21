package main

import (
	"bytes"
	"context"
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
// The test's own context is the parent, so finishing or cancelling the test ends the child first.
func cliCmdDeadline(t *testing.T, deadline time.Duration, bin string, args ...string) *exec.Cmd {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), deadline)
	cmd := exec.CommandContext(ctx, bin, args...)
	processGroup(cmd)
	cmd.WaitDelay = waitDelayAfterKill
	cmd.Env = cliEnv(t)
	// Run already waited on a completed child; a child that was started but never waited for is
	// still ours to reap. The group kill runs either way, because the parent can exit before a
	// descendant that inherited its pipes.
	t.Cleanup(func() {
		cancel()
		killGroup(cmd)
		if cmd.Process != nil && cmd.ProcessState == nil {
			_ = cmd.Wait()
		}
	})
	return cmd
}

// cliEnv builds a child environment with extra entries winning, the way os/exec resolves duplicates.
// The temp root is the ceiling: a temp directory can still be inside a repository owned by the
// operator's home, and that repository is not the change any CLI test asked about (#483, #502).
func cliEnv(t *testing.T, extra ...string) []string {
	t.Helper()
	root, err := filepath.Abs(os.TempDir())
	if err != nil {
		t.Fatalf("temp root %q: %v", os.TempDir(), err)
	}
	env := make([]string, 0, len(os.Environ())+len(extra)+1)
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if inheritedGitScopeVar(name) {
			continue
		}
		env = append(env, entry)
	}
	env = append(env, "GIT_CEILING_DIRECTORIES="+root)
	return append(env, extra...)
}

func inheritedGitScopeVar(name string) bool {
	switch name {
	case "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY",
		"GIT_COMMON_DIR", "GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_NAMESPACE",
		"GIT_CEILING_DIRECTORIES":
		return true
	}
	return name == "GIT_CONFIG" || strings.HasPrefix(name, "GIT_CONFIG_")
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
