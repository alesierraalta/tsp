package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The study's report shape (#372, #384): the defect named in prose, in a report the plan never got.
const reportNamingADefect = "Lo pinné con un test que falla hoy.\n\n**Defecto:** `middleware/decompress.go:85` compara `Content-Encoding` sin ignorar mayúsculas.\n"

func reportPlanDir(t *testing.T, bin string) (dir, plan string) {
	t.Helper()
	dir = t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	plan = filepath.Join(dir, "plan.md")
	if out, code := runCLI(t, bin, "plan", "init", "--micro", "--path", plan); code != 0 {
		t.Fatalf("plan init = %d\n%s", code, out)
	}
	return dir, plan
}

func writeReport(t *testing.T, dir, text string) string {
	t.Helper()
	path := filepath.Join(dir, "report.md")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// A report that names a defect no Findings row records makes plan check fail and name the citation; once the
// finding is recorded the same report passes with the output plan check gives today.
func TestPlanCheckReportRefusesADefectOnlyTheReportNames(t *testing.T) {
	bin := buildCLI(t)
	dir, plan := reportPlanDir(t, bin)
	report := writeReport(t, dir, reportNamingADefect)

	out, code := runCLI(t, bin, "plan", "check", "--path", plan, "--report", report)
	if code != 1 || !strings.Contains(out, "report cites middleware/decompress.go:85, which no Findings row records") || strings.Contains(out, "well formed") {
		t.Fatalf("plan check --report = %d\n%s", code, out)
	}

	if out, code := runCLI(t, bin, "plan", "add-evidence", "--path", plan, "--claim", "encoding compared case-sensitively",
		"--executed", "ran the decompress test", "--admit", "go test ./middleware", "--inputs", "gzip header GZIP",
		"--observed", "the body was not decompressed", "--label", "observado"); code != 0 {
		t.Fatalf("add-evidence = %d\n%s", code, out)
	}
	if out, code := runCLI(t, bin, "plan", "add-finding", "--path", plan, "--location", "middleware/decompress.go:85",
		"--severity", "silently wrong answer", "--data-safe", "yes", "--evidence", "E1", "--status", "confirmed",
		"--test", "middleware/decompress_test.go :: TestDecompressIgnoresCase", "--verdict-by", "tsp / 2026-11-07", "--reason", "seen"); code != 0 {
		t.Fatalf("add-finding = %d\n%s", code, out)
	}
	plain, plainCode := runCLI(t, bin, "plan", "check", "--path", plan)
	out, code = runCLI(t, bin, "plan", "check", "--path", plan, "--report", report)
	if code != 0 || plainCode != 0 || out != plain {
		t.Fatalf("a recorded report must give today's output: --report %d %q, plain %d %q", code, out, plainCode, plain)
	}
}

// --report - reads the report from stdin, and a report that cites only tests and docs passes unchanged.
func TestPlanCheckReportReadsStdinAndIgnoresTestsAndDocs(t *testing.T) {
	bin := buildCLI(t)
	_, plan := reportPlanDir(t, bin)
	plain, plainCode := runCLI(t, bin, "plan", "check", "--path", plan)

	cmd := exec.Command(bin, "plan", "check", "--path", plan, "--report", "-")
	cmd.Stdin = strings.NewReader("Tests in `middleware/decompress_test.go:40`; docs in `README.md:3`.\n")
	got, err := cmd.CombinedOutput()
	if err != nil || string(got) != plain || plainCode != 0 {
		t.Fatalf("stdin report citing tests and docs = %v %q, want today's %d %q", err, got, plainCode, plain)
	}

	cmd = exec.Command(bin, "plan", "check", "--path", plan, "--report", "-")
	cmd.Stdin = strings.NewReader(reportNamingADefect)
	got, err = cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(got), "middleware/decompress.go:85") {
		t.Fatalf("stdin report naming a defect = %v %q, want a refusal", err, got)
	}
}

func TestPlanCheckReportNamesAnUnreadableReport(t *testing.T) {
	bin := buildCLI(t)
	_, plan := reportPlanDir(t, bin)
	out, code := runCLI(t, bin, "plan", "check", "--path", plan, "--report", filepath.Join(t.TempDir(), "missing.md"))
	if code != 1 || !strings.Contains(out, "read report") {
		t.Fatalf("missing report = %d\n%s", code, out)
	}
}
