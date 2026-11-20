package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// telemetryRunIn is telemetryRun with the command's working directory set, so the domain events can be
// observed against a real repository without changing the process's own directory.
func telemetryRunIn(t *testing.T, dir, bin, home, stdin string, env []string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), append([]string{"TSP_HOME=" + home, "TPP_HOME=", "RDD_PLUS_HOME=", "TSP_RUN_ID="}, env...)...)
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	var ee *exec.ExitError
	switch {
	case err == nil:
		return stdout.String(), stderr.String(), 0
	case errors.As(err, &ee):
		return stdout.String(), stderr.String(), ee.ExitCode()
	}
	t.Fatalf("run %v in %s: %v", args, dir, err)
	return "", "", -1
}

func domainBody(t *testing.T, event map[string]any, kind string) map[string]any {
	t.Helper()
	body, ok := event[kind].(map[string]any)
	if !ok {
		t.Fatalf("%s event carries no %s body: %v", kind, kind, event)
	}
	return body
}

func stringSlice(t *testing.T, value any) []string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("%v is not a string array: %v", value, err)
	}
	return out
}
