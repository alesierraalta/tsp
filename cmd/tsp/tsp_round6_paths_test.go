package main

import (
	"os"
	"strings"
	"testing"
)

// A --paths that names nothing is a usage error. Split into no path at all, it falls through to the
// whole tree's verdict, so the caller that asked a scoped question is handed the answer to the legacy
// one and cannot tell the difference.
func TestCheckPathsRefusesAnEmptyValue(t *testing.T) {
	for _, raw := range []string{"", " ", ",", " , "} {
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
