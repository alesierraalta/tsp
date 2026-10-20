package bench

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTSPSessionAppliesDeletedSourceFile(t *testing.T) {
	repo, commit := upstreamRepo(t)
	source := sourceCase(t, repo, commit)
	patch := "--- a/calc.go\n+++ /dev/null\n@@ -1,3 +0,0 @@\n-package calc\n-\n-func Add(a, b int) int { return a - b }\n" +
		"--- /dev/null\n+++ b/replacement.go\n@@ -0,0 +1,3 @@\n+package calc\n+\n+func Add(a, b int) int { return a + b }\n"
	writeFile(t, source, "fix.patch", patch)
	caseDir, err := MaterializeCase(source, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ws := t.TempDir()
	if err := copyTree(filepath.Join(caseDir, FixtureDir), ws); err != nil {
		t.Fatal(err)
	}
	writeFile(t, ws, "agent_test.go", "package calc\nimport \"testing\"\nfunc TestWitness(t *testing.T) {}\n")
	if err := applySessionFix(caseDir, ws, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(ws, "agent_test.go")); err != nil {
		t.Fatalf("agent test not retained: %v", err)
	}
	if _, err := os.Stat(filepath.Join(ws, "replacement.go")); err != nil {
		t.Fatalf("reference addition not applied: %v", err)
	}
	if _, err := os.Stat(filepath.Join(ws, "calc.go")); !os.IsNotExist(err) {
		t.Fatalf("reference deletion not applied: calc.go still present (stat error %v)", err)
	}
}

func TestTSPOracleSimilarNamesRemainIndependent(t *testing.T) {
	c := CatchResult{CaughtBy: map[string][]string{"D1": {"example.com/demo.TestAB"}}}
	names := oracleNames("agent_test.go", []byte("package demo; import \"testing\"; func TestA(t *testing.T) {}; func TestAB(t *testing.T) {}"), c)
	found := false
	for _, name := range names {
		if name == "TestA" || name == "example.com/demo.TestA" {
			found = true
		}
	}
	if !found {
		t.Fatalf("TestA omitted from independent judgments: %v", names)
	}
}

func TestTSPOracleJSDescribeWitnessCanBeDropped(t *testing.T) {
	run, opts := oracleReading(t)
	ws := filepath.Join(run, "demo/1/ws")
	// Keep the existing Go file as an unrelated control; add a real Jest-style named witness.
	if err := os.WriteFile(filepath.Join(ws, "agent.test.js"), []byte("describe('outer', () => { test('fails', () => {}); });"), 0644); err != nil {
		t.Fatal(err)
	}
	r := Result{Case: "demo", Run: 1, Workspace: ws, Catch: CatchResult{Checked: true, Caught: map[string]bool{"D1": true}, CaughtBy: map[string][]string{"D1": {"outer fails"}}, TestFiles: []string{"agent.test.js"}}}
	data, err := json.Marshal(Aggregate{Cases: []Result{r}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(run, "aggregate.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
	opts.Agent = func(_ context.Context, _ string, _ Key, o Options) (AgentResult, error) {
		marker := "Named tests (JSON): "
		i := strings.LastIndex(o.Prompt, marker)
		if i < 0 {
			return AgentResult{}, fmt.Errorf("no named tests")
		}
		var names []string
		if err := json.Unmarshal([]byte(o.Prompt[i+len(marker):]), &names); err != nil {
			return AgentResult{}, err
		}
		tests := make([]OracleTest, 0, len(names))
		for _, name := range names {
			tests = append(tests, OracleTest{Name: name, Verdict: "invented", Reason: "not in contract"})
		}
		answer, err := json.Marshal(struct {
			Tests []OracleTest `json:"tests"`
		}{tests})
		return AgentResult{Result: string(answer)}, err
	}
	report, err := VerifyOracle(run, opts)
	if err != nil {
		t.Fatal(err)
	}
	if report.Recorded.Caught != 1 || report.Filtered.Caught != 0 {
		t.Fatalf("invented Jest witness not dropped: recorded=%+v filtered=%+v files=%+v", report.Recorded, report.Filtered, report.Files)
	}
}

func TestTSPOracleOutputSymlinkCannotRewriteReading(t *testing.T) {
	run, opts := oracleReading(t)
	original, err := os.ReadFile(filepath.Join(run, "aggregate.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(run, "oracle"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../aggregate.json", filepath.Join(run, "oracle/verdicts.json")); err != nil {
		t.Fatal(err)
	}
	opts.Agent = func(context.Context, string, Key, Options) (AgentResult, error) {
		return AgentResult{Result: `{"tests":[{"name":"TestContract","verdict":"contract","reason":"documented"},{"name":"TestInvented","verdict":"unclear","reason":"unspecified"}]}`}, nil
	}
	_, _ = VerifyOracle(run, opts) // Refusal or safe output is acceptable; rewriting the reading is not.
	after, err := os.ReadFile(filepath.Join(run, "aggregate.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(original) {
		t.Fatal("verify-oracle rewrote original aggregate.json through oracle/verdicts.json symlink")
	}
}
