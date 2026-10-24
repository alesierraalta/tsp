package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/alesierraalta/tsp/internal/verify"
)

// verifyTestRepo builds a repository with one commit and one unstaged change, so runVerify has a
// candidate to freeze without touching anything outside t.TempDir.
func verifyTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.name", "tsp test"},
		{"config", "user.email", "test@example.com"},
	} {
		verifyGit(t, dir, args...)
	}
	path := filepath.Join(dir, "main.go")
	if err := os.WriteFile(path, []byte("package main\n\nfunc one() int { return 1 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	verifyGit(t, dir, "add", "-A")
	verifyGit(t, dir, "commit", "-q", "-m", "base")
	if err := os.WriteFile(path, []byte("package main\n\nfunc one() int { return 2 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func verifyGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

// verifySession writes a session command that answers with verdict.
func verifySession(t *testing.T, verdict string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "session.sh")
	body := "#!/bin/sh\ncat > \"$1/verdict.json\" <<'JSON'\n" + verdict + "\nJSON\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestVerifyExitCodes pins the contract a caller reads: only a pinned verdict with no findings is 0,
// findings are 1, and a verifier that cannot answer is 2 — never 0 by default.
func TestVerifyExitCodes(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		verdict string
		want    int
	}{
		{name: "no session command", want: 2},
		{name: "pinned", verdict: `{"verdict":"pinned"}`, want: 0},
		{name: "unpinned with a finding", verdict: `{"verdict":"unpinned","findings":[{"file":"main.go","line":3,"claim":"the test passes on the base commit","evidence":"redcheck on the base commit exits 0"}]}`, want: 1},
		{name: "pinned with a finding", verdict: `{"verdict":"pinned","findings":[{"file":"main.go","line":3,"claim":"a layer was never planned","evidence":"no database test was written"}]}`, want: 1},
		{name: "malformed verdict", verdict: `{"verdict":"looks fine"}`, want: 2},
		{name: "no verdict", want: 2, args: []string{"--session-cmd", "/bin/true"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := verifyTestRepo(t)
			args := append([]string{"--cwd", repo}, tc.args...)
			if tc.verdict != "" {
				args = append(args, "--session-cmd", verifySession(t, tc.verdict))
			}
			if got := runVerify(args); got != tc.want {
				t.Fatalf("runVerify = %d, want %d", got, tc.want)
			}
			artifact := filepath.Join(repo, ".tsp-verify.json")
			_, err := os.Stat(artifact)
			if tc.want == 2 && !os.IsNotExist(err) {
				t.Fatal("an unusable verifier must not publish an artifact")
			}
			if tc.want != 2 && err != nil {
				t.Fatalf("a usable verifier must publish the artifact: %v", err)
			}
		})
	}
}

// A host that reviews the change itself (RDD) writes the verdict document and TSP publishes it: the
// RDD flow must work on a machine where the standalone session command never runs.
func TestVerifyPublishesAHostVerdict(t *testing.T) {
	dir := t.TempDir()
	session := filepath.Join(dir, "must-not-run")
	body := `{"verdict":"pinned","findings":[],"notes":"RDD admitted the review"}`
	if err := os.WriteFile(filepath.Join(dir, verify.VerdictFile), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if code := runVerify([]string{"--publish", dir, "--base", "abc1234", "--session-cmd", session, "--json"}); code != 2 {
		t.Fatalf("publish with a session command = exit %d, want 2", code)
	}
	if code := runVerify([]string{"--publish", dir, "--base", "abc1234", "--json"}); code != 0 {
		t.Fatalf("published pinned verdict = exit %d, want 0", code)
	}
	if _, err := os.Stat(filepath.Join(dir, verify.ArtifactName)); err != nil {
		t.Fatalf("artifact: %v", err)
	}
}
