package bench

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A run that changed a test and left the plan empty owes a recording; one that recorded a finding, or changed no
// test, owes nothing, and a plan that cannot be read is not nudged because nothing would land (#398).
func TestNeedsRecordNudge(t *testing.T) {
	dir := asChangeCase(t)
	ws := filepath.Join(t.TempDir(), "ws")
	if _, err := ScaffoldAsChange(dir, ws, Key{Suite: "true"}, time.Minute); err != nil {
		t.Fatal(err)
	}
	plan := filepath.Join(ws, PlanPath)
	if err := os.MkdirAll(filepath.Dir(plan), 0o755); err != nil {
		t.Fatal(err)
	}
	empty := "# Plan\n\n## Findings\n\n| Id | Finding (path:line, one line) | Status |\n|---|---|---|\n"
	if err := os.WriteFile(plan, []byte(empty), 0o644); err != nil {
		t.Fatal(err)
	}
	if needsRecordNudge(ws) {
		t.Fatal("no test changed yet, so nothing is owed")
	}
	if err := os.WriteFile(filepath.Join(ws, "src/avg_test.go"), []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !needsRecordNudge(ws) {
		t.Fatal("a changed test with an empty plan owes a recording")
	}
	withRow := empty + "| F1 | src/avg.go:1 wrong | open |\n"
	if err := os.WriteFile(plan, []byte(withRow), 0o644); err != nil {
		t.Fatal(err)
	}
	if needsRecordNudge(ws) {
		t.Fatal("a recorded finding owes nothing")
	}
	if err := os.Remove(plan); err != nil {
		t.Fatal(err)
	}
	if !needsRecordNudge(ws) {
		t.Fatal("a run with no plan at all owes a recording")
	}
}

func TestChangedTestsSeesTestFilesOnly(t *testing.T) {
	dir := asChangeCase(t)
	ws := filepath.Join(t.TempDir(), "ws")
	if _, err := ScaffoldAsChange(dir, ws, Key{Suite: "true"}, time.Minute); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"src/avg_test.go", "src/avg.go", "docs/README.md"} {
		full := filepath.Join(ws, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got := changedTests(ws)
	if len(got) != 1 || got[0] != "src/avg_test.go" {
		t.Fatalf("changedTests = %v, want only the test file", got)
	}
}
