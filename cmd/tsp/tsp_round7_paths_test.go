package main

import (
	"os"
	"strings"
	"testing"
)

// --paths answers from the tree and never runs a suite. Accepting --suite beside it discarded an
// explicit argument while the command reported a completed answer, so the caller believed the suite it
// asked for had decided.
func TestCheckPathsRefusesToSilentlyIgnoreASuite(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	old := os.Stderr
	os.Stderr = f
	code := runCheck([]string{"--cwd", t.TempDir(), "--paths", "src/app.go", "--suite", "go test ./..."})
	os.Stderr = old
	b, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	const want = "check: --paths answers for one change; --suite runs the changed tests for the whole change"
	if code != 2 {
		t.Fatalf("code=%d stderr=%q, want a usage error", code, b)
	}
	if got := strings.TrimSpace(string(b)); got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
}
