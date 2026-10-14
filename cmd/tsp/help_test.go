package main

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// Asking for help is not a mistake. An agent reading `tsp plan add-evidence --help` to learn the flags got
// exit 2 and read it as a failed call; the usage is the answer, so it exits 0. A flag the command does not
// know is still a usage error.
func TestHelpFlagExitsZeroAndAnUnknownFlagStaysAUsageError(t *testing.T) {
	bin := buildCLI(t)
	dir := t.TempDir()
	run := func(args ...string) (int, string) {
		cmd := exec.Command(bin, args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode(), string(out)
		}
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		return 0, string(out)
	}
	for _, sub := range [][]string{
		{"plan", "add-evidence"}, {"plan", "init"}, {"feedback"}, {"doctor"}, {"check"},
		{"bench", "run"}, {"bench", "adjudicate"}, {"sync"},
	} {
		for _, help := range []string{"-h", "--help"} {
			code, out := run(append(append([]string{}, sub...), help)...)
			if code != 0 {
				t.Errorf("%s %s exited %d, want 0", strings.Join(sub, " "), help, code)
			}
			if !strings.Contains(out, "Usage of") {
				t.Errorf("%s %s printed no usage:\n%s", strings.Join(sub, " "), help, out)
			}
		}
		if code, _ := run(append(append([]string{}, sub...), "--no-such-flag")...); code != 2 {
			t.Errorf("%s --no-such-flag exited %d, want 2", strings.Join(sub, " "), code)
		}
	}
}
