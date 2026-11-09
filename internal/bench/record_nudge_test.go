package bench

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The two nudge flags are the contract the analysis reads from result.json: a run that was asked carries both keys
// when the plan ended with a finding, and a run that was never asked carries neither (#398).
func TestResultCarriesTheNudgeFlags(t *testing.T) {
	raw, err := json.Marshal(Result{Case: "nudged", Run: 1, RecordNudgeAsked: true, RecordedAfterNudge: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"record_nudge_asked":true`, `"recorded_after_nudge":true`} {
		if !strings.Contains(string(raw), want) {
			t.Fatalf("a nudged result lacks %s: %s", want, raw)
		}
	}
	zero, err := json.Marshal(Result{Case: "quiet", Run: 1})
	if err != nil {
		t.Fatal(err)
	}
	for _, absent := range []string{"record_nudge_asked", "recorded_after_nudge"} {
		if strings.Contains(string(zero), absent) {
			t.Fatalf("a run that was never nudged carries %s: %s", absent, zero)
		}
	}
}

// The plan held no finding when the nudge was sent, so a finding afterwards is the recording: a workspace with no plan
// or an empty Findings table recorded nothing (#398).
func TestRecordedAfterNudge(t *testing.T) {
	ws := t.TempDir()
	if recordedAfterNudge(ws) {
		t.Fatal("a workspace with no plan recorded nothing")
	}
	plan := filepath.Join(ws, PlanPath)
	if err := os.MkdirAll(filepath.Dir(plan), 0o755); err != nil {
		t.Fatal(err)
	}
	empty := "# Plan\n\n## Findings\n\n| Id | Finding (path:line, one line) | Status |\n|---|---|---|\n"
	if err := os.WriteFile(plan, []byte(empty), 0o644); err != nil {
		t.Fatal(err)
	}
	if recordedAfterNudge(ws) {
		t.Fatal("an empty Findings table recorded nothing")
	}
	if err := os.WriteFile(plan, []byte(empty+"| F1 | src/avg.go:1 wrong | open |\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !recordedAfterNudge(ws) {
		t.Fatal("a finding row after the nudge is the recording")
	}
}

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
