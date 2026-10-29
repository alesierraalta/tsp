package main

import (
	"strings"
	"testing"
)

// --json says it prints the verdict as one JSON object, and a caller that reads the check
// programmatically has nothing else to go on. A usage error still has to come back in that shape:
// the path from a rejected flag to a confident "nothing to report" is exactly the one that turns a
// typo into silence.
func TestJSONAnswersAUsageErrorAsAVerdict(t *testing.T) {
	t.Setenv("TSP_GENTLE_OBSERVATION", "")
	dir := committedUnit(t, map[string]string{"a.go": "package sample\n\nfunc F() int { return 1 }\n"})

	code, out, errOut := runCheckCapturing(t, "--cwd", dir, "--json", "--since", " ", "--paths", "a.go")
	if code != 2 {
		t.Fatalf("a --since that names nothing is a usage error: code=%d stdout=%q stderr=%q", code, out, errOut)
	}
	if !strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Fatalf("--json must answer one JSON object even when it refuses the flags: %q", out)
	}
	if v := readVerdict(t, out); v.Decision != "error" {
		t.Fatalf("the verdict must say the check could not answer: %+v", v)
	}
}
