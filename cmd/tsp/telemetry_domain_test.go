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

// A check that ran in a real repository reports the decision, the reason category, the changed
// languages and the runners they imply, and leaves the command's stdout and exit exactly as they were.
func TestTelemetryCheckEvent(t *testing.T) {
	bin := buildCLI(t)
	repo := pathsRepo(t, "main.go")
	writeIn(t, repo, "main.go", "package sample\n\nfunc F() int { return 2 }\n\nfunc G() int { return 3 }\n")
	writeIn(t, repo, "app.js", "export function run() {\n  return 1\n}\n")
	t.Setenv("TSP_GENTLE_OBSERVATION", "")
	args := []string{"check", "--cwd", ".", "--paths", "main.go", "--paths", "app.js"}

	offHome := t.TempDir()
	plainOut, plainErr, plainCode := telemetryRunIn(t, repo, bin, offHome, "", nil, args...)
	if plainCode != 1 {
		t.Fatalf("check exit = %d\n%s%s", plainCode, plainOut, plainErr)
	}
	if _, err := os.Stat(ledgerPath(offHome)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("telemetry off wrote a ledger (err %v)", err)
	}

	home := t.TempDir()
	enableTelemetry(t, bin, home)
	out, errOut, code := telemetryRunIn(t, repo, bin, home, "", nil, args...)
	if code != plainCode || out != plainOut || errOut != plainErr {
		t.Fatalf("telemetry on changed the check: exit %d (want %d)\nstdout %q (want %q)\nstderr %q (want %q)",
			code, plainCode, out, plainOut, errOut, plainErr)
	}

	checks := eventsOfKind(readEvents(t, home), "check")
	if len(checks) != 1 {
		t.Fatalf("got %d check events, want 1", len(checks))
	}
	body := domainBody(t, checks[0], "check")
	if body["decision"] != "ask" || body["exit"] != 1.0 || body["reason"] != "no_witness" {
		t.Errorf("check body = %v, want decision ask, exit 1, reason no_witness", body)
	}
	if got, want := stringSlice(t, body["languages"]), []string{"go", "js"}; !equalStrings(got, want) {
		t.Errorf("languages = %v, want %v", got, want)
	}
	if got, want := stringSlice(t, body["runners"]), []string{"go test", "node --test"}; !equalStrings(got, want) {
		t.Errorf("runners = %v, want %v", got, want)
	}
	if body["files"] != 2.0 {
		t.Errorf("files = %v, want 2", body["files"])
	}
	if lines, _ := body["lines"].(float64); lines <= 0 {
		t.Errorf("lines = %v, want the changed line count", body["lines"])
	}
	for _, absent := range []string{"waiver", "opt_out"} {
		if _, ok := body[absent]; ok {
			t.Errorf("an unmeasured %s must stay absent: %v", absent, body)
		}
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Plan and findings command results are domain events too: the plan reports its action and row
// counts, and a findings check reports its action and the number of findings it reviewed.
func TestTelemetryPlanAndFindingsEvents(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	enableTelemetry(t, bin, home)
	repo := pathsRepo(t, "main.go")

	if out, errOut, code := telemetryRunIn(t, repo, bin, home, "", nil, "plan", "init", "--path", "plan.md"); code != 0 {
		t.Fatalf("plan init = %d\n%s%s", code, out, errOut)
	}
	if out, errOut, code := telemetryRunIn(t, repo, bin, home, "", nil, "plan", "check", "--path", "plan.md"); code != 0 {
		t.Fatalf("plan check = %d\n%s%s", code, out, errOut)
	}
	if out, errOut, code := telemetryRunIn(t, repo, bin, home, "", nil, "findings", "check", "--cwd", "."); code != 0 {
		t.Fatalf("findings check = %d\n%s%s", code, out, errOut)
	}

	plans := eventsOfKind(readEvents(t, home), "plan")
	if len(plans) != 2 {
		t.Fatalf("got %d plan events, want init and check: %v", len(plans), plans)
	}
	var actions []string
	for _, event := range plans {
		actions = append(actions, domainBody(t, event, "plan")["action"].(string))
	}
	if !equalStrings(actions, []string{"init", "check"}) {
		t.Errorf("plan actions = %v, want [init check]", actions)
	}
	initBody := domainBody(t, plans[0], "plan")
	if _, ok := initBody["findings"]; !ok {
		t.Errorf("plan init body lacks findings count: %v", initBody)
	}
	if _, ok := initBody["evidence"]; !ok {
		t.Errorf("plan init body lacks evidence count: %v", initBody)
	}
	checkBody := domainBody(t, plans[1], "plan")
	if checkBody["problems"] != 0.0 {
		t.Errorf("a fresh plan check found problems = %v, want 0", checkBody["problems"])
	}

	finds := eventsOfKind(readEvents(t, home), "finding")
	if len(finds) != 1 {
		t.Fatalf("got %d finding events, want one findings check: %v", len(finds), finds)
	}
	finding := domainBody(t, finds[0], "finding")
	if finding["action"] != "check" || finding["count"] != 0.0 {
		t.Errorf("finding body = %v, want action check with count 0", finding)
	}
}

// A refused class is the caller's text, not a known family: it must never reach the ledger.
func TestTelemetryFindingsRecordNeverStoresARefusedClass(t *testing.T) {
	bin := buildCLI(t)
	home := t.TempDir()
	enableTelemetry(t, bin, home)
	_, _, code := telemetryRun(t, bin, home, "", nil, "findings", "record", "--cwd", t.TempDir(), "--class", "customer acme password leak")
	if code != 2 {
		t.Fatalf("unknown class exit = %d, want 2", code)
	}
	raw, _ := os.ReadFile(ledgerPath(home))
	if strings.Contains(string(raw), "acme") {
		t.Fatalf("a refused class reached the ledger:\n%s", raw)
	}
	findings := eventsOfKind(readEvents(t, home), "finding")
	if len(findings) != 1 {
		t.Fatalf("got %d finding events, want the refused record noted once without its class", len(findings))
	}
	if _, ok := findings[0]["finding"].(map[string]any)["class"]; ok {
		t.Errorf("refused record kept a class: %v", findings[0])
	}
}
