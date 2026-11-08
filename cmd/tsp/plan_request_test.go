package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlanRequestCLI(t *testing.T) {
	bin := buildCLI(t)
	dir := t.TempDir()
	spec := filepath.Join(dir, "request.txt")
	path := filepath.Join(dir, "plan.md")
	text := "Deliver the feature\n"
	if err := os.WriteFile(spec, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	if out, code := runCLI(t, bin, "plan", "init", "--spec", spec, "--path", path); code != 0 {
		t.Fatalf("init %d: %s", code, out)
	}
	if out, code := runCLI(t, bin, "plan", "check", "--path", path); code != 0 {
		t.Fatalf("check %d: %s", code, out)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), text) {
		t.Fatal("request missing")
	}
	updated := strings.Replace(string(data), "|---|---|---|---|---|", "|---|---|---|---|---|\n| C1 | Deliver | covered | E99 | - |", 1)
	if err := os.WriteFile(path, []byte(updated), 0600); err != nil {
		t.Fatal(err)
	}
	if out, code := runCLI(t, bin, "plan", "check", "--path", path); code != 1 || !strings.Contains(out, "C1") {
		t.Fatalf("check %d: %s", code, out)
	}
	stdinPath := filepath.Join(dir, "stdin.md")
	cmd := exec.Command(bin, "plan", "init", "--spec", "-", "--path", stdinPath)
	cmd.Stdin = strings.NewReader(text)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("stdin init: %v %s", err, out)
	}
	data, err = os.ReadFile(stdinPath)
	if err != nil || !strings.Contains(string(data), text) {
		t.Fatalf("stdin missing: %v", err)
	}
}

// A criterion answered by a row that exists is what the section is for: it must pass, with --micro and --force too.
func TestRequestCriteriaAcceptsACoveredCriterionWithARealSource(t *testing.T) {
	bin := buildCLI(t)
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	spec := filepath.Join(dir, "spec.md")
	if err := os.WriteFile(spec, []byte("The change must not drop the header.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := filepath.Join(dir, "plan.md")
	if out, code := runCLI(t, bin, "plan", "init", "--micro", "--spec", spec, "--path", plan); code != 0 {
		t.Fatalf("init --micro --spec = %d %s", code, out)
	}
	if out, code := runCLI(t, bin, "plan", "init", "--spec", spec, "--path", plan, "--force"); code != 0 {
		t.Fatalf("init --force = %d %s", code, out)
	}
	if out, code := runCLI(t, bin, "plan", "add-evidence", "--path", plan, "--claim", "the header survives", "--executed", "ran it",
		"--admit", "go test ./...", "--inputs", "a request with a header", "--observed", "the header stayed", "--label", "observado"); code != 0 {
		t.Fatalf("add-evidence = %d %s", code, out)
	}
	body, _ := os.ReadFile(plan)
	withCriterion := strings.Replace(string(body), "|---|---|---|---|---|\n", "|---|---|---|---|---|\n| C1 | the header survives | covered | E1 | - |\n", 1)
	if err := os.WriteFile(plan, []byte(withCriterion), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, code := runCLI(t, bin, "plan", "check", "--path", plan); code != 0 {
		t.Fatalf("a covered criterion with a real source must pass: %d %s", code, out)
	}
	if out, code := runCLI(t, bin, "plan", "gaps", "--path", plan); strings.Contains(out, "request criterion owed") {
		t.Fatalf("a covered criterion must not be owed: %d %s", code, out)
	}
}
