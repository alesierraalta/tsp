package bench

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// goSubpackageCase is a Go module whose only defect lives in the model subpackage, while the case's
// suite names the root package alone, as a real case's scoped suite does.
func goSubpackageCase(t *testing.T, suite string) (string, Key) {
	t.Helper()
	caseDir := t.TempDir()
	files := map[string]string{
		"go.mod":         "module example.com/sub\n\ngo 1.21\n",
		"root.go":        "package sub\n\nimport \"example.com/sub/model\"\n\nfunc Order() string { return model.Order() }\n",
		"root_test.go":   "package sub\n\nimport \"testing\"\n\nfunc TestRootBuilds(t *testing.T) {}\n",
		"model/model.go": "package model\n\nfunc Order() string { return \"cab\" }\n",
	}
	for dir, order := range map[string]string{FixtureDir: "cab", filepath.Join(FixDir, "all"): "abc"} {
		for rel, body := range files {
			if rel == "model/model.go" {
				body = "package model\n\nfunc Order() string { return \"" + order + "\" }\n"
			}
			full := filepath.Join(caseDir, dir, rel)
			if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	return caseDir, Key{Suite: suite, Defects: []Defect{{ID: "D1", File: "model/model.go", Line: 3}}}
}

// The catch check runs the case's suite, and a suite scoped to some packages never runs a test the agent
// placed in another one: that test was scored as "all green, not caught" without ever executing. The
// packages holding the agent's tests run too, placed before the suite's own flags, because go test reads
// a package named after a flag as an argument for the test binary and silently skips it.
func TestDiscriminateRunsAgentTestsOutsideTheSuitePackages(t *testing.T) {
	if testing.Short() {
		t.Skip("runs go test")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not installed")
	}
	const modelTest = "package model\n\nimport \"testing\"\n\nfunc TestOrderIsSorted(t *testing.T) {\n\tif got := Order(); got != \"abc\" {\n\t\tt.Fatalf(\"Order() = %q, want abc\", got)\n\t}\n}\n"
	const rootTest = "package sub\n\nimport \"testing\"\n\nfunc TestRootOrderIsSorted(t *testing.T) {\n\tif got := Order(); got != \"abc\" {\n\t\tt.Fatalf(\"Order() = %q, want abc\", got)\n\t}\n}\n"
	cases := []struct {
		name, suite, testFile, test, caughtBy string
	}{
		{"a test in a suite package", "go test .", "agent_test.go", rootTest, "example.com/sub.TestRootOrderIsSorted"},
		{"a test outside the suite packages", "go test .", "model/agent_test.go", modelTest, "example.com/sub/model.TestOrderIsSorted"},
		{"a test outside a suite that ends in flags", "go test . -skip '^TestNothing$'", "model/agent_test.go", modelTest, "example.com/sub/model.TestOrderIsSorted"},
		{"a test outside a suite whose flag value looks like a package", "go test -coverpkg ./... .", "model/agent_test.go", modelTest, "example.com/sub/model.TestOrderIsSorted"},
		{"a test outside a suite with a build flag value", "go test . -buildmode default", "model/agent_test.go", modelTest, "example.com/sub/model.TestOrderIsSorted"},
		{"a test outside a suite with a test.-prefixed flag", "go test . -test.run Test", "model/agent_test.go", modelTest, "example.com/sub/model.TestOrderIsSorted"},
		{"a test outside a suite that passes -args", "go test . -args custom", "model/agent_test.go", modelTest, "example.com/sub/model.TestOrderIsSorted"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caseDir, key := goSubpackageCase(t, tc.suite)
			ws := agentWorkspace(t, caseDir, map[string]string{tc.testFile: tc.test})
			got := Discriminate(caseDir, ws, key, 2*time.Minute)
			if !got.Checked || !got.AllGreen {
				t.Fatalf("checked=%v allGreen=%v notes=%v", got.Checked, got.AllGreen, got.Notes)
			}
			if !got.Caught["D1"] {
				t.Fatalf("caught = %v, want D1 caught; notes=%v", got.Caught, got.Notes)
			}
			if by := got.CaughtBy["D1"]; len(by) != 1 || by[0] != tc.caughtBy {
				t.Fatalf("caught by = %v, want [%s]", got.CaughtBy["D1"], tc.caughtBy)
			}
		})
	}
}
