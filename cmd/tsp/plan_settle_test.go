package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The repair lands, the witness that was red on the defective code passes, and the row stops reading confirmed:
// this is the chain that used to dead-end. --pins refused the stale row and named no status that could move it,
// and nothing could rewrite the row, so the plan reported a repaired defect as pending forever (#509).
func TestPlanSettleMarksTheRepairedFindingFixed(t *testing.T) {
	bin := buildCLI(t)
	dir := pinRepo(t)
	plan := filepath.Join(dir, "plan.md")
	addRow(t, bin, dir, "calc/red_test.go :: TestAddFails")

	if out, code := runCLI(t, bin, "plan", "check", "--path", plan, "--pins"); code != 0 {
		t.Fatalf("a witness must be red while the defect is there: %d %s", code, out)
	}
	repair := "package calc\n\nfunc Add(a, b int) int { return a + b }\n"
	if err := os.WriteFile(filepath.Join(dir, "calc/calc.go"), []byte(repair), 0o644); err != nil {
		t.Fatal(err)
	}

	out, code := runCLI(t, bin, "plan", "check", "--path", plan, "--pins")
	if code != 1 || !strings.Contains(out, "passes on this code, so it pins nothing") {
		t.Fatalf("the repaired defect's witness must be refused as stale: %d %s", code, out)
	}
	if !strings.Contains(out, "tsp plan settle --id F1 --status fixed") {
		t.Fatalf("the refusal must name the remedy that settles the row: %s", out)
	}

	out, code = runCLI(t, bin, "plan", "settle", "--path", plan, "--id", "F1", "--status", "fixed")
	if code != 0 || !strings.Contains(out, "settled F1") {
		t.Fatalf("settle = %d %s", code, out)
	}
	raw, err := os.ReadFile(plan)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "| fixed |") {
		t.Fatalf("the row still does not read fixed: %s", raw)
	}
	if out, code := runCLI(t, bin, "plan", "check", "--path", plan, "--pins"); code != 0 || !strings.Contains(out, "well formed") {
		t.Fatalf("a settled row must pass --pins: %d %s", code, out)
	}
	// The reproduction the row was recorded with still runs: settle moves a verdict, it does not drop the evidence.
	if out, code := runCLI(t, bin, "plan", "check", "--path", plan); code != 0 {
		t.Fatalf("settle broke the plan: %d %s", code, out)
	}
}

// A settle that cannot land leaves the plan byte-identical: an id the plan does not hold is its refusal (1), and
// a value the vocabulary does not carry is a malformed invocation (2).
func TestPlanSettleRefusesWithoutMovingThePlan(t *testing.T) {
	bin := buildCLI(t)
	dir := pinRepo(t)
	plan := filepath.Join(dir, "plan.md")
	addRow(t, bin, dir, "calc/red_test.go :: TestAddFails")
	before, err := os.ReadFile(plan)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		args []string
		code int
		want string
	}{
		{"an id the plan does not hold", []string{"--id", "F9", "--status", "fixed"}, 1, "F9"},
		{"a status outside the vocabulary", []string{"--id", "F1", "--status", "resolved"}, 2, "open, confirmed, fixed, gap-closed, rejected, wontfix"},
		{"no id", []string{"--status", "fixed"}, 2, "--id"},
		{"no status", []string{"--id", "F1"}, 2, "--status"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, code := runCLI(t, bin, append([]string{"plan", "settle", "--path", plan}, tc.args...)...)
			if code != tc.code || !strings.Contains(out, tc.want) {
				t.Fatalf("settle = %d, want %d naming %q: %s", code, tc.code, tc.want, out)
			}
			now, err := os.ReadFile(plan)
			if err != nil {
				t.Fatal(err)
			}
			if string(now) != string(before) {
				t.Fatal("a refusal moved the plan")
			}
		})
	}
}
