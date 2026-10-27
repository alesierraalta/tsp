package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"strings"
	"testing"
)

// `tsp findings next` prints the loop's answer as text, or as exactly one JSON object for a host, and exits with
// the answer's code; an argument it does not take is a usage error.
func TestFindingsNextPrintsOneAnswer(t *testing.T) {
	t.Setenv("TSP_HOME", t.TempDir())
	bin := buildCLI(t)
	dir := pathsRepo(t, "a.go")
	branch, err := exec.Command("git", "-C", dir, "symbolic-ref", "--short", "HEAD").Output()
	if err != nil {
		t.Fatal(err)
	}
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
	want := "no TSP round is open on " + strings.TrimSpace(string(branch)) + ": run the TSP flow, then `tsp findings record`"
	if code, out, errOut := run("findings", "next", "--cwd", dir); code != 2 || out != "error: "+want+"\n" || errOut != "" {
		t.Fatalf("text: exit=%d stdout=%q stderr=%q", code, out, errOut)
	}
	code, out, errOut := run("findings", "next", "--cwd", dir, "--json")
	decoder := json.NewDecoder(strings.NewReader(out))
	var answer struct {
		Next   string            `json:"next"`
		Reason string            `json:"reason"`
		Items  []json.RawMessage `json:"items"`
		Scope  *json.RawMessage  `json:"scope"`
	}
	if err := decoder.Decode(&answer); err != nil {
		t.Fatalf("not one JSON object: %v\n%q", err, out)
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("more than one JSON value on stdout: %q", out)
	}
	if code != 2 || errOut != "" || answer.Next != "error" || answer.Reason != want || answer.Items == nil || len(answer.Items) != 0 || answer.Scope != nil {
		t.Fatalf("json: exit=%d stderr=%q answer=%+v\n%s", code, errOut, answer, out)
	}
	code, out, errOut = run("findings", "next", "--cwd", dir, "extra")
	if code != 2 || out != "" || !strings.Contains(errOut, "Usage of findings:") || !strings.Contains(errOut, "tsp findings next [--cwd <dir>] [--timeout <duration>] [--json]") {
		t.Fatalf("argument: exit=%d stdout=%q stderr=%q", code, out, errOut)
	}
}
