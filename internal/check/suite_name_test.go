package check

import "testing"

// D2: a suite is a closed runner name, never the caller's command or a path.
func TestSuiteNameIsAClosedRunnerName(t *testing.T) {
	cases := map[string]string{
		"go test ./...":      "go test",
		"pytest -q":          "pytest",
		"node --test tests/": "node --test",
		"jest --runInBand":   "jest",
		"vitest run":         "vitest run",
		"npx jest":           "npx jest",
		"npx vitest run":     "npx vitest run",
		"cargo test --all":   "cargo test",
		"/tmp/secret-runner": "other",
		"python secret.py":   "other",
	}
	for suite, want := range cases {
		if got := suiteName(suite); got != want {
			t.Errorf("suiteName(%q) = %q, want %q", suite, got, want)
		}
	}
	if got := suiteName(""); got != "" {
		t.Errorf("suiteName(\"\") = %q, want an unmeasured empty value", got)
	}
}
