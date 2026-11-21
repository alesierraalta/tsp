package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestProbeQualityReportsOnARealChange drives the probe over a real repository: a complex function and a
// copied block inside the change exit 1, and a change the probe cannot measure exits 2 instead of passing.
// One fixture carries both violations on purpose: it proves the command wires the rules into an exit code.
// Each rule is pinned on its own in internal/quality (TestComplexityCountsOnlyWhatTheChangeTouches and its
// duplication sibling), so a rule breaking there fails there, not here.
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

// The usage has to name the boundary: an enclosing repository must not be silently substituted for
// the directory the caller gave.
func TestProbeQualityHelpNamesTheCwdBoundary(t *testing.T) {
	var help bytes.Buffer
	parseQualityFlags([]string{"--help"}, &help)
	for _, phrase := range []string{"--cwd", "repository root", "outside"} {
		if !strings.Contains(help.String(), phrase) {
			t.Fatalf("help does not say %q:\n%s", phrase, help.String())
		}
	}
}

// A directory below a repository is not that repository: the probe must say so instead of taking the
// enclosing repository's change as the one the caller asked about.
func TestProbeQualityRefusesARepositoryAboveCwd(t *testing.T) {
	top := t.TempDir()
	verifyGit(t, top, "init", "-q")
	if err := os.WriteFile(filepath.Join(top, "app.go"), []byte("package app\n\nfunc F() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inner := filepath.Join(top, "nested")
	if err := os.Mkdir(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadDir(top)
	if err != nil {
		t.Fatal(err)
	}

	bin := buildCLI(t)
	cmd := cliCmd(t, bin, "probe", "quality", "--cwd", inner)
	// Let git see the enclosing repository here; the refusal is the production contract, not the
	// test helper's ceiling. The helper's extra env entry wins over its default ceiling.
	cmd.Env = cliEnv(t, "GIT_CEILING_DIRECTORIES="+filepath.Dir(top))
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
	if code != 2 || !strings.Contains(stderr.String(), "outside") {
		t.Fatalf("probe quality --cwd %s = %d, stderr %q\nwant exit 2 naming a repository outside the directory", inner, code, stderr.String())
	}
	after, err := os.ReadDir(top)
	if err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatalf("the refused probe touched the enclosing repository: before %d entries, after %d", len(before), len(after))
	}
}
