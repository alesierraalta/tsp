package scan

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sarifReport is the shape semgrep, gitleaks and trivy all emit: tool name, rule id, level, message,
// and a physical location. The parser reads nothing else.
const sarifReport = `{
  "version": "2.1.0",
  "runs": [{
    "tool": {"driver": {"name": "semgrep"}},
    "results": [
      {"ruleId": "python.lang.security.audit.exec", "level": "error",
       "message": {"text": "Detected use of exec"},
       "locations": [{"physicalLocation": {"artifactLocation": {"uri": "app/main.py"}, "region": {"startLine": 12}}}]},
      {"ruleId": "python.lang.best-practice.print", "level": "note",
       "message": {"text": "print statement"},
       "locations": [{"physicalLocation": {"artifactLocation": {"uri": "vendor/lib.py"}, "region": {"startLine": 3}}}]}
    ]
  }]
}`

func TestSarifIngestionKeepsTheChangeAndDropsTheRest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.sarif")
	if err := os.WriteFile(path, []byte(sarifReport), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := ReadSarif(path, []string{"app/main.py"})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(rep.Findings) != 1 {
		t.Fatalf("findings %+v, want only the one on the changed file", rep.Findings)
	}
	got := rep.Findings[0]
	if got.Tool != "semgrep" || got.File != "app/main.py" || got.Line != 12 || got.Level != "error" || got.Rule == "" || got.Message == "" {
		t.Fatalf("finding = %+v, want tool, file, line, level, rule and message", got)
	}
	if !rep.Blocking("error") {
		t.Fatal("an error finding must block at level error")
	}
	if rep.Blocking("critical") {
		t.Fatal("no finding is above a level the report does not use")
	}
}

// An empty list of changed files means the caller is not scoping: everything the report says counts.
func TestSarifIngestionWithoutScopeKeepsEverything(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.sarif")
	if err := os.WriteFile(path, []byte(sarifReport), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := ReadSarif(path, nil)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(rep.Findings) != 2 {
		t.Fatalf("findings = %d, want both when no scope was named", len(rep.Findings))
	}
}

// A change that touched nothing puts nothing in scope: the report may say everything, and none of it
// belongs to this change.
func TestSarifIngestionWithAnEmptyScopeKeepsNothing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "report.sarif")
	if err := os.WriteFile(path, []byte(sarifReport), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := ReadSarif(path, []string{})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(rep.Findings) != 0 {
		t.Fatalf("findings = %+v, want none when the change touched nothing", rep.Findings)
	}
}

func TestMissingEngineIsUnusable(t *testing.T) {
	_, err := Run(context.Background(), Options{
		Cwd:  "/repo",
		Deps: Deps{Git: fakeGit("app/main.py\n"), LookPath: func(string) (string, error) { return "", errors.New("missing") }},
	})
	if !errors.Is(err, ErrUnusable) || !strings.Contains(err.Error(), "gitleaks") {
		t.Fatalf("err = %v, want an unusable run naming the engine to install", err)
	}
}

func fakeGit(changed string) func(string, ...string) (string, error) {
	return func(dir string, args ...string) (string, error) {
		if args[0] == "rev-parse" {
			return "/repo\n", nil
		}
		return changed, nil
	}
}

// scanFake writes the SARIF the parser reads wherever the engine was told to put it.
func scanFake(t *testing.T) func(context.Context, string, string, ...string) (string, error) {
	t.Helper()
	return func(_ context.Context, _ string, name string, args ...string) (string, error) {
		for i, a := range args {
			if a == "--output" || a == "--report-path" {
				if i+1 < len(args) {
					return "", os.WriteFile(args[i+1], []byte(sarifReport), 0o644)
				}
			}
		}
		return "", nil
	}
}

// A scan that only ran gitleaks has read secrets, not rules: the report names the engines that ran, so
// "no findings" cannot pass for "no rule looked".
func TestScanRunsSemgrepBesideGitleaks(t *testing.T) {
	report, err := Run(context.Background(), Options{
		Cwd:  "/repo",
		Base: "base",
		Deps: Deps{
			Git:      fakeGit("app/main.py\n"),
			LookPath: func(name string) (string, error) { return "/usr/bin/" + name, nil },
			Run:      scanFake(t),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Engine != "gitleaks+semgrep" || len(report.Notes) != 0 {
		t.Fatalf("engine = %q notes = %v, want both engines and no note", report.Engine, report.Notes)
	}
	if len(report.Findings) == 0 {
		t.Fatal("want the rule the report carries, so a merge that drops findings fails here")
	}
}

// Without semgrep the scan still reports, and says so: silence here reads as a clean rules pass.
func TestScanWithoutSemgrepSaysOnlySecretsWereRead(t *testing.T) {
	report, err := Run(context.Background(), Options{
		Cwd:  "/repo",
		Base: "base",
		Deps: Deps{
			Git: fakeGit("app/main.py\n"),
			LookPath: func(name string) (string, error) {
				if name == "semgrep" {
					return "", errors.New("missing")
				}
				return "/usr/bin/" + name, nil
			},
			Run: scanFake(t),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Notes) != 1 || !strings.Contains(report.Notes[0], "only secrets were scanned") {
		t.Fatalf("notes = %v, want the missing-engine note", report.Notes)
	}
}
