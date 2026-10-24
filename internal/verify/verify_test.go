package verify

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// newRepo builds a repository with one commit holding files, so a candidate can be written on top.
func newRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.name", "tsp test"},
		{"config", "user.email", "test@example.com"},
	} {
		runGit(t, dir, args...)
	}
	for name, body := range files {
		writeFile(t, filepath.Join(dir, name), body)
	}
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "base")
	return dir
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return string(out)
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// sessionScript writes the verifier session the tests drive: a shell script that receives the
// package directory as its last argument, exactly like a real session command.
func sessionScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "session.sh")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// verdictScript is the common case: a session that answers with one fixed verdict.
func verdictScript(t *testing.T, verdict string) string {
	t.Helper()
	return sessionScript(t, "cat > \"$1/"+VerdictFile+"\" <<'JSON'\n"+verdict+"\nJSON\n")
}

func changedRepo(t *testing.T) string {
	t.Helper()
	repo := newRepo(t, map[string]string{"main.go": "package main\n\nfunc double(x int) int { return x + 1 }\n"})
	writeFile(t, filepath.Join(repo, "main.go"), "package main\n\nfunc double(x int) int { return x * 2 }\n")
	return repo
}

func TestRunFreezesTheCandidateReadOnlyAndPublishesTheVerdict(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can write through a read-only tree")
	}
	repo := changedRepo(t)
	session := sessionScript(t, `
if touch "$TSP_VERIFY_FROZEN/probe" 2>/dev/null; then
  echo "frozen tree is writable" >&2
  exit 9
fi
if [ ! -f "$TSP_VERIFY_FROZEN/main.go" ]; then
  echo "frozen tree is missing the candidate" >&2
  exit 8
fi
grep -q 'x \* 2' "$TSP_VERIFY_FROZEN/main.go" || { echo "frozen tree is not the candidate" >&2; exit 7; }
cat > "$1/`+VerdictFile+`" <<'JSON'
{"verdict":"pinned","notes":"tests fail on the base commit"}
JSON
`)

	result, err := Run(context.Background(), Options{Cwd: repo, SessionCmd: session, Task: "double the value"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Verdict.Verdict != VerdictPinned {
		t.Fatalf("verdict = %q, want pinned", result.Verdict.Verdict)
	}
	if result.Blocking() {
		t.Fatal("a pinned verdict with no findings must not block")
	}
	data, err := os.ReadFile(filepath.Join(repo, ArtifactName))
	if err != nil {
		t.Fatalf("artifact: %v", err)
	}
	var artifact struct {
		Verdict string `json:"verdict"`
		Blind   bool   `json:"blind"`
	}
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatalf("artifact is not JSON: %v", err)
	}
	if artifact.Verdict != VerdictPinned || !artifact.Blind {
		t.Fatalf("artifact = %+v, want a blind pinned verdict", artifact)
	}
	if _, err := os.Stat(result.FrozenDir); !os.IsNotExist(err) {
		t.Fatalf("frozen tree still exists at %s unless Keep was set", result.FrozenDir)
	}
}

func TestRunPackageCarriesOnlyTheDocuments(t *testing.T) {
	repo := changedRepo(t)
	listing := filepath.Join(t.TempDir(), "listing.txt")
	session := sessionScript(t, "cat > \"$1/"+VerdictFile+"\" <<'JSON'\n{\"verdict\":\"pinned\"}\nJSON\nls -1 \"$1\" > "+listing+"\n")

	if _, err := Run(context.Background(), Options{Cwd: repo, SessionCmd: session}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	data, err := os.ReadFile(listing)
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	got := strings.Fields(string(data))
	want := []string{DiffFile, CheckFile, RedcheckFile, TaskFile, VerdictFile}
	if len(got) != len(want) {
		t.Fatalf("package holds %v, want exactly %v", got, want)
	}
	for i, name := range want {
		if got[i] != name {
			t.Fatalf("package holds %v, want exactly %v", got, want)
		}
	}
}

func TestRunRecordsOutputsThatWereNotProduced(t *testing.T) {
	repo := changedRepo(t)
	peek := filepath.Join(t.TempDir(), "check.txt")
	session := sessionScript(t, "cp \"$1/"+CheckFile+"\" "+peek+"\ncat > \"$1/"+VerdictFile+"\" <<'JSON'\n{\"verdict\":\"pinned\"}\nJSON\n")

	if _, err := Run(context.Background(), Options{Cwd: repo, SessionCmd: session}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	data, err := os.ReadFile(peek)
	if err != nil {
		t.Fatalf("check copy: %v", err)
	}
	if !strings.Contains(string(data), "check was not run") {
		t.Fatalf("%s = %q, want it to say check was not run", CheckFile, data)
	}
}

func TestRunBlocksOnFindings(t *testing.T) {
	repo := changedRepo(t)
	session := verdictScript(t, `{"verdict":"unpinned","findings":[{"file":"main.go","line":3,"claim":"the test passes on the base commit","evidence":"redcheck on the base commit exits 0"}]}`)

	result, err := Run(context.Background(), Options{Cwd: repo, SessionCmd: session})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !result.Blocking() || len(result.Verdict.Findings) != 1 {
		t.Fatalf("result = %+v, want one blocking finding", result.Verdict)
	}
}

func TestRunFailsClosedWhenTheVerifierNeverAnswers(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		session bool
	}{
		{name: "no session command"},
		{name: "no verdict", session: true, body: "echo nothing here\n"},
		{name: "malformed verdict", session: true, body: "printf 'not json' > \"$1/" + VerdictFile + "\"\n"},
		{name: "unknown verdict", session: true, body: "printf '{\"verdict\":\"maybe\"}' > \"$1/" + VerdictFile + "\"\n"},
		{name: "rejection without evidence", session: true, body: "printf '{\"verdict\":\"unpinned\"}' > \"$1/" + VerdictFile + "\"\n"},
		{name: "finding without evidence", session: true, body: "printf '{\"verdict\":\"pinned\",\"findings\":[{\"file\":\"main.go\",\"line\":1,\"claim\":\"c\"}]}' > \"$1/" + VerdictFile + "\"\n"},
		{name: "session exits non-zero", session: true, body: "exit 3\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := changedRepo(t)
			opts := Options{Cwd: repo}
			if tc.session {
				opts.SessionCmd = sessionScript(t, tc.body)
			}
			_, err := Run(context.Background(), opts)
			if !errors.Is(err, ErrUnusable) {
				t.Fatalf("err = %v, want ErrUnusable", err)
			}
			if _, statErr := os.Stat(filepath.Join(repo, ArtifactName)); !os.IsNotExist(statErr) {
				t.Fatal("an unusable verifier must not publish an artifact")
			}
		})
	}
}

func TestRunKeepsTheFrozenTreeOnlyWhenAsked(t *testing.T) {
	repo := changedRepo(t)
	session := verdictScript(t, `{"verdict":"pinned"}`)

	result, err := Run(context.Background(), Options{Cwd: repo, SessionCmd: session, Keep: true})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(func() {
		_, _ = exec.Command("git", "-C", repo, "worktree", "remove", "--force", result.FrozenDir).CombinedOutput()
		makeWritable(result.FrozenDir)
	})
	if !result.Kept {
		t.Fatal("Keep: true must be reported in the result")
	}
	if _, err := os.Stat(filepath.Join(result.FrozenDir, "main.go")); err != nil {
		t.Fatalf("frozen tree was not kept: %v", err)
	}
}
