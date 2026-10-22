package main

import (
	"os"
	"strings"
	"testing"
)

// A --paths occurrence that names nothing is a usage error. An empty or blank value trims to no
// path, and falling through to the whole tree's verdict would hand the caller that asked a scoped
// question the answer to the legacy one. Comma-only values were removed from this table: a comma is
// now a legal path character, and such a value fails loudly by name in the repository tests.
func TestCheckPathsRefusesAnEmptyValue(t *testing.T) {
	for _, raw := range []string{"", " "} {
		t.Run("raw="+raw, func(t *testing.T) {
			f, err := os.CreateTemp(t.TempDir(), "stderr")
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			old := os.Stderr
			os.Stderr = f
			code := runCheck([]string{"--cwd", t.TempDir(), "--paths", raw})
			os.Stderr = old
			b, err := os.ReadFile(f.Name())
			if err != nil {
				t.Fatal(err)
			}
			if code != 2 {
				t.Fatalf("code=%d stderr=%q", code, b)
			}
			if !strings.Contains(string(b), "--paths") {
				t.Fatalf("the refusal must name the flag: %q", b)
			}
		})
	}
}
