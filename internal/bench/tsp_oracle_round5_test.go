package bench

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// setOracleCatch replaces the stored catch record of a reading, so a round-5 probe can drive
// VerifyOracle with the exact red tests, names and counts the defect needs.
func setOracleCatch(t *testing.T, run string, catch CatchResult) {
	t.Helper()
	path := filepath.Join(run, "aggregate.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var agg Aggregate
	if err := json.Unmarshal(data, &agg); err != nil {
		t.Fatal(err)
	}
	agg.Cases[0].Catch = catch
	out, err := json.Marshal(agg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, out, 0644); err != nil {
		t.Fatal(err)
	}
}

// A verdict is subtracted from one run, but a bare Go test name is shared by every package that
// defines it. When package a's TestParse is invented and package b's TestParse is contract, a red
// test reported against b is still explained by the contract and must stay.
func TestOracleDropsARedTestOnlyWhenEveryJudgedNameWasInvented(t *testing.T) {
	for _, other := range []string{"contract", "invented"} {
		t.Run("package b judged "+other, func(t *testing.T) {
			run, opts := oracleReading(t)
			ws := filepath.Join(run, "demo/1/ws")
			// The reading's default test file defines the same bare name; each package keeps its own.
			if err := os.Remove(filepath.Join(ws, "agent_test.go")); err != nil {
				t.Fatal(err)
			}
			write := func(rel, body string) {
				t.Helper()
				p := filepath.Join(ws, rel)
				if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte(body), 0644); err != nil {
					t.Fatal(err)
				}
			}
			write("a/a_test.go", "package a\nimport \"testing\"\nfunc TestParse(t *testing.T) {}\n")
			write("b/b_test.go", "package b\nimport \"testing\"\nfunc TestParse(t *testing.T) {}\n")
			setOracleCatch(t, run, CatchResult{
				Checked:     true,
				TestFiles:   []string{"a/a_test.go", "b/b_test.go"},
				Caught:      map[string]bool{},
				BrokenTests: 1,
				RedTests:    []string{"example.com/demo/b.TestParse"},
			})
			opts.Agent = func(_ context.Context, _ string, _ Key, o Options) (AgentResult, error) {
				switch {
				case strings.Contains(o.Prompt, "Test file: a_test.go"):
					return AgentResult{Result: `{"tests":[{"name":"TestParse","verdict":"invented","reason":"not in the contract"}]}`}, nil
				case strings.Contains(o.Prompt, "Test file: b_test.go"):
					return AgentResult{Result: `{"tests":[{"name":"TestParse","verdict":"` + other + `","reason":"judged"}]}`}, nil
				}
				return AgentResult{}, errors.New("unexpected judged file in prompt: " + o.Prompt)
			}
			report, err := VerifyOracle(run, opts)
			if err != nil {
				t.Fatal(err)
			}
			if report.Recorded.Broken != 1 {
				t.Fatalf("recorded broken = %d, want 1", report.Recorded.Broken)
			}
			want := 1
			if other == "invented" {
				want = 0
			}
			if report.Filtered.Broken != want {
				t.Fatalf("filtered broken = %d, want %d: a verdict from another file of the same run was applied to package b's TestParse", report.Filtered.Broken, want)
			}
		})
	}
}

// An answer this run's own prompt produced but the parser rejects is not a legacy verdict: the
// summary must say how many tests a rejected answer kept, or a reading that lost most verdicts
// reads like one judged by an older prompt.
func TestOracleSummaryNamesRejectedAnswersOfThisRun(t *testing.T) {
	run, opts := oracleReading(t)
	opts.Agent = func(_ context.Context, _ string, _ Key, o Options) (AgentResult, error) {
		if !strings.Contains(o.Prompt, "TestInvented") {
			return AgentResult{}, errors.New("prompt has no named tests: " + o.Prompt)
		}
		// One test for two named ones: the answer is rejected as a wrong test count.
		return AgentResult{Result: `{"tests":[{"name":"TestContract","verdict":"contract","reason":"documented"}]}`}, nil
	}
	report, err := VerifyOracle(run, opts)
	if err != nil {
		t.Fatal(err)
	}
	if report.Verdicts["unclear"] != 2 {
		t.Fatalf("verdicts = %v, want both tests kept unclear", report.Verdicts)
	}
	stored, err := os.ReadFile(filepath.Join(run, "oracle", "verdicts.json"))
	if err != nil {
		t.Fatal(err)
	}
	var persisted map[string]any
	if err := json.Unmarshal(stored, &persisted); err != nil {
		t.Fatal(err)
	}
	counts, ok := persisted["verdict_counts"].(map[string]any)
	if !ok || counts["unclear"] != float64(2) {
		t.Fatalf("verdict_counts = %v, want the unclear key kept for compatibility", persisted["verdict_counts"])
	}
	summary, err := os.ReadFile(filepath.Join(run, "oracle", "summary.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(summary), "older prompt") {
		t.Fatalf("this run's rejected answer is described as an older prompt:\n%s", summary)
	}
	lower := strings.ToLower(string(summary))
	if !strings.Contains(lower, "unparseable") {
		t.Fatalf("summary names neither unparseable answers nor this run:\n%s", summary)
	}
	if !regexp.MustCompile(`(unparseable|rejected)[^.\n]*\b2\b`).MatchString(lower) {
		t.Fatalf("summary does not state how many tests a rejected answer kept:\n%s", summary)
	}
}

// A kept run whose workspace has no added or changed test file has nothing to judge, but its case
// does have a contract: that is not a reading that measured nothing, so it must publish rather
// than refuse with a message about a missing contract.
func TestOraclePublishesAReadingWithNothingToJudge(t *testing.T) {
	run, opts := oracleReading(t)
	if err := os.Remove(filepath.Join(run, "demo/1/ws/agent_test.go")); err != nil {
		t.Fatal(err)
	}
	setOracleCatch(t, run, CatchResult{
		Checked:  true,
		Caught:   map[string]bool{"D1": true},
		CaughtBy: map[string][]string{"D1": {"TestContract"}},
	})
	opts.Agent = func(context.Context, string, Key, Options) (AgentResult, error) {
		return AgentResult{}, errors.New("invoked a judge for a reading with no test file to judge")
	}
	report, err := VerifyOracle(run, opts)
	if err != nil {
		t.Fatalf("a kept run with nothing to judge was refused as if its case had no contract: %v", err)
	}
	if report.Unjudged != 0 || len(report.UnjudgedCases) != 0 {
		t.Fatalf("unjudged = %d %v, want a case whose contract exists to stay judged", report.Unjudged, report.UnjudgedCases)
	}
	if report.Recorded != report.Filtered {
		t.Fatalf("recorded = %+v, filtered = %+v, want unchanged views when nothing was judged", report.Recorded, report.Filtered)
	}
	if report.Recorded.Caught != 1 {
		t.Fatalf("caught = %d, want the reading's own catch record kept", report.Recorded.Caught)
	}
	if _, err := os.Stat(filepath.Join(run, "oracle", "verdicts.json")); err != nil {
		t.Fatalf("no report was published: %v", err)
	}
}
