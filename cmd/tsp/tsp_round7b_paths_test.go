package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func captureCheck(t *testing.T, args []string) (int, string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = w, w
	code := runCheck(args)
	w.Close()
	os.Stdout, os.Stderr = oldOut, oldErr
	out, err := io.ReadAll(r)
	r.Close()
	if err != nil {
		t.Fatal(err)
	}
	return code, string(out)
}

// --cwd is "the directory inside the repository to check". Handed a directory that cannot be read at
// all — one that does not exist, or a file — git fails there, and the command answers "not a git
// repository: nothing to check" with exit 0. That is a false diagnosis and a false clean at once: the
// process may well stand in a repository whose source is changed, and the verifier reads exit 0 as
// "this change owes no test". A cwd the check could not read earns a usage error, never a verdict.
func TestCheckPathsDoesNotClearAChangeWhenTheCwdCannotBeRead(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "src_app.js")
	if err := os.WriteFile(file, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		cwd  string
	}{
		{"a directory that does not exist", filepath.Join(dir, "missing")},
		{"a file", file},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, out := captureCheck(t, []string{"--cwd", tc.cwd, "--paths", "src_app.js"})
			if code == 0 {
				t.Fatalf("cwd=%s exit = 0, want a usage error, not the answer that owes nothing:\n%s", tc.cwd, out)
			}
			if strings.Contains(out, "nothing to check") {
				t.Fatalf("cwd=%s was answered as a clean tree instead of refused:\n%s", tc.cwd, out)
			}
		})
	}
}
