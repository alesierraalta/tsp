package mutate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// strykerJson is the shape Stryker's json reporter writes, cut to what the adapter reads.
const strykerJson = `{"files":{"src/price.js":{"mutants":[
  {"mutatorName":"EqualityOperator","status":"Killed","location":{"start":{"line":2}}},
  {"mutatorName":"ConditionalExpression","status":"Survived","location":{"start":{"line":2}}},
  {"mutatorName":"ArithmeticOperator","status":"NoCoverage","location":{"start":{"line":3}}},
  {"mutatorName":"StringLiteral","status":"CompileError","location":{"start":{"line":3}}}
]},"src/other.js":{"mutants":[
  {"mutatorName":"StringLiteral","status":"Survived","location":{"start":{"line":9}}}
]}}}`

// jsRepo lays out a repository with Stryker's config in place.
func jsRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "stryker.conf.json"), []byte(`{"testRunner":"vitest"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "reports", "mutation"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, jsReportPath), []byte(strykerJson), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "price.js"), []byte("x"), 0o644); err != nil && !os.IsNotExist(err) {
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// jsDeps answers the engine call without running it, so the adapter is judged on its normalization.
func jsDeps(root, changed string) Deps {
	return Deps{
		Git: func(_ string, args ...string) (string, error) {
			for _, a := range args {
				if a == "rev-parse" {
					return root + "\n", nil
				}
			}
			return " M " + changed, nil
		},
		LookPath: func(string) (string, error) { return "/fake/npx", nil },
		Run:      func(context.Context, string, string, ...string) (string, error) { return "", nil },
	}
}

// A JavaScript change is measured and normalized, and the report only speaks about the changed files.
func TestJavaScriptChangeIsMeasuredByStryker(t *testing.T) {
	root := jsRepo(t)
	report, err := Run(context.Background(), Options{Cwd: root, MaxMutants: 10, Deps: jsDeps(root, "src/price.js")})
	if err != nil {
		t.Fatal(err)
	}
	if report.Engine != "stryker" || report.MutationCount != 4 {
		t.Fatalf("report = %+v, want four mutants of the changed file only", report)
	}
	if report.Killed != 1 || report.Survived != 1 || report.NotCovered != 1 || report.NotViable != 1 {
		t.Fatalf("counters = %+v, want 1 killed, 1 survived, 1 not covered, 1 not viable", report)
	}
	if len(report.Survivors) != 1 || report.Survivors[0].Line != 2 || report.Survivors[0].File != "src/price.js" {
		t.Fatalf("survivors = %+v, want src/price.js:2", report.Survivors)
	}
}

// Without a Stryker config the run names the file to write instead of failing inside the engine.
func TestJavaScriptWithoutConfigNamesTheFileToWrite(t *testing.T) {
	root := t.TempDir()
	_, err := Run(context.Background(), Options{Cwd: root, MaxMutants: 10, Deps: jsDeps(root, "src/price.js")})
	if !errors.Is(err, ErrUnusable) || !strings.Contains(err.Error(), "stryker.conf.json") {
		t.Fatalf("err = %v, want the config instruction", err)
	}
}

// The engine's scratch and the tests themselves are never targets.
func TestJavaScriptTargetsSkipTestsAndInstalledPackages(t *testing.T) {
	got := jsTargets([]string{"src/price.js", "src/price.test.js", "node_modules/x/index.js", "src/__tests__/a.ts", "reports/mutation/mutation.json"})
	if len(got) != 1 || got[0] != "src/price.js" {
		t.Fatalf("targets = %v, want only src/price.js", got)
	}
}
