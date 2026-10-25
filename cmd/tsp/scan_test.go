package main

import (
	"os"
	"path/filepath"
	"testing"
)

const scanSarif = `{"version":"2.1.0","runs":[{"tool":{"driver":{"name":"fake"}},"results":[
{"ruleId":"secret-in-source","level":"error","message":{"text":"hardcoded credential"},
 "locations":[{"physicalLocation":{"artifactLocation":{"uri":"main.go"},"region":{"startLine":3}}}]},
{"ruleId":"note-on-untouched-file","level":"error","message":{"text":"not this change"},
 "locations":[{"physicalLocation":{"artifactLocation":{"uri":"other.go"},"region":{"startLine":1}}}]}]}]}`

func scanReport(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "report.sarif")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// A finding on a file the change touched is the answer, and the finding on an untouched file is not.
func TestScanReportsTheChangeAndExitsOne(t *testing.T) {
	dir := verifyTestRepo(t)
	if code := runScan([]string{"--cwd", dir, "--sarif", scanReport(t, scanSarif)}); code != 1 {
		t.Fatalf("exit = %d, want 1 when the changed file has a finding", code)
	}
}

// Nothing at the gated level is not the same as nothing scanned: clean exits 0, unmeasured exits 2.
func TestScanWithoutEngineExitsTwo(t *testing.T) {
	dir := verifyTestRepo(t)
	t.Setenv("PATH", "/usr/bin:/bin")
	if code := runScan([]string{"--cwd", dir}); code != 2 {
		t.Fatalf("exit = %d, want 2 when no scanner is installed", code)
	}
}

// A level this package does not know is refused instead of silently gating at some other level.
func TestScanRefusesAnUnknownLevel(t *testing.T) {
	dir := verifyTestRepo(t)
	if code := runScan([]string{"--cwd", dir, "--sarif", scanReport(t, scanSarif), "--level", "critical"}); code != 2 {
		t.Fatalf("exit = %d, want 2 for an unknown level", code)
	}
}
