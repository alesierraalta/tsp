package main

import (
	"bytes"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// The defect class travels from the command line to the registry layer: an unknown one is refused before the round is
// judged, naming the classes that exist, and the usage line documents the flag.
func TestFindingsRecordClassIsWired(t *testing.T) {
	t.Setenv("TSP_HOME", t.TempDir())
	bin := buildCLI(t)
	dir := pathsRepo(t, "a.go")
	run := func(args ...string) (int, string, string) {
		cmd := exec.Command(bin, args...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			return exit.ExitCode(), stdout.String(), stderr.String()
		}
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		return 0, stdout.String(), stderr.String()
	}
	code, out, errOut := run("findings", "record", "--cwd", dir, "--class", "vibes")
	if code != 2 || errOut != "" {
		t.Fatalf("unknown class: exit=%d stdout=%q stderr=%q", code, out, errOut)
	}
	if !strings.Contains(out, `unknown defect class "vibes"`) || !strings.Contains(out, "concurrency") {
		t.Fatalf("refusal %q must name the class and the ones that exist", out)
	}
	if code, out, errOut := run("findings"); code != 2 || out != "" || !strings.Contains(errOut, "--class <familia>") {
		t.Fatalf("usage: exit=%d stdout=%q stderr=%q", code, out, errOut)
	}
}
