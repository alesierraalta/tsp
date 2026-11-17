// Package testutil holds helpers shared by the product's own tests.
package testutil

import (
	"path/filepath"
	"testing"
)

// CeilingGitAt stops git's upward repository discovery at root for the rest of t. A directory
// below root cannot see a repository above it, even when TMPDIR itself lives inside one (#483).
// Git does not apply a ceiling to the directory it starts from, so pass the parent of any
// directory that must read as "outside a repository".
func CeilingGitAt(t *testing.T, root string) {
	t.Helper()
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("git ceiling %q: %v", root, err)
	}
	t.Setenv("GIT_CEILING_DIRECTORIES", abs)
}
