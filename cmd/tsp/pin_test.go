package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPinImportsReviewFindings(t *testing.T) {
	dir := t.TempDir()
	plan := filepath.Join(dir, "plan.md")
	findings := filepath.Join(dir, "review.json")
	if err := os.WriteFile(plan, []byte("# Plan\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(findings, []byte(`{"findings":[{"location":"src/foo.go:12","claim":"wrong result","severity":"high"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	out, code := runCLI(t, buildCLI(t), "pin", "--path", plan, "--findings", findings)
	if code != 0 {
		t.Fatalf("import failed: %d %s", code, out)
	}
	body, err := os.ReadFile(plan)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "| P1 | src/foo.go:12 | wrong result | open | - | - |") {
		t.Fatalf("missing finding: %s", body)
	}
}
