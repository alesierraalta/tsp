package main

import (
	"strings"
	"testing"
)

// With feedback disabled the retrospective cannot be recorded, so the template says so up front instead of
// handing out a skeleton the --file call will refuse: hard-suite runs spent three calls (template, fill,
// refused file) learning what one call can tell them. Asking for the template is not an error, so it exits 0.
func TestFeedbackTemplateSaysSoWhenFeedbackIsDisabled(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	configDir := t.TempDir()

	out, code := runCLIWithHomeEnv(t, home, bin, "feedback", "--config-dir", configDir, "--template")
	if code != 0 {
		t.Fatalf("disabled template exit = %d, want 0\n%s", code, out)
	}
	for _, want := range []string{"feedback is disabled", "skip the retrospective", "tsp feature enable feedback"} {
		if !strings.Contains(out, want) {
			t.Errorf("disabled template missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "verdict:") {
		t.Errorf("a disabled template must not print the skeleton:\n%s", out)
	}

	if out, code = runCLIWithHomeEnv(t, home, bin, "feature", "enable", "feedback"); code != 0 {
		t.Fatalf("enable feedback = %d %q", code, out)
	}
	out, code = runCLIWithHomeEnv(t, home, bin, "feedback", "--config-dir", configDir, "--template")
	if code != 0 || !strings.Contains(out, "verdict:") {
		t.Fatalf("enabled template = %d, want the skeleton\n%s", code, out)
	}
}
