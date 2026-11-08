package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlanReplay(t *testing.T) {
	bin := buildCLI(t)
	dir := pinRepo(t)
	path := filepath.Join(dir, "plan.md")
	addRow(t, bin, dir, "calc/red_test.go :: TestAddFails")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, command, want string
		code                int
	}{
		{"unproven", "", "finding F1 has no reproduction, so its claim is unproven", 0},
		{"passing", "true", "reproduction passes", 1},
		{"failing", "false", "well formed", 0},
		{"missing", "tsp_missing_reproduction_command_399", "could not start", 1},
		{"timeout", "sleep 2", "timed out", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			text := strings.Replace(string(original), "| r | - |  |  |", "| r | - |  | "+tc.command+" |", 1)
			if tc.command != "" && text == string(original) {
				t.Fatal("finding row not found")
			}
			if err := os.WriteFile(path, []byte(text), 0600); err != nil {
				t.Fatal(err)
			}
			out, code := runCLI(t, bin, "plan", "check", "--path", path, "--replay", "--replay-timeout", "500ms")
			if code != tc.code || !strings.Contains(out, tc.want) {
				t.Fatalf("got %d %s", code, out)
			}
		})
	}
}

func TestPlanCheckDoesNotReplayByDefault(t *testing.T) {
	bin := buildCLI(t)
	dir := pinRepo(t)
	path := filepath.Join(dir, "plan.md")
	addRow(t, bin, dir, "calc/red_test.go :: TestAddFails")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Replace(string(raw), "| r | - |  |  |", "| r | - |  | touch marker |", 1)
	if text == string(raw) {
		t.Fatal("finding row not found")
	}
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	out, code := runCLI(t, bin, "plan", "check", "--path", path)
	if code != 0 {
		t.Fatalf("%d %s", code, out)
	}
	if _, err := os.Stat(filepath.Join(dir, "marker")); !os.IsNotExist(err) {
		t.Fatalf("command ran: %v", err)
	}
}
