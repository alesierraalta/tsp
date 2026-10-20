package bench

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOracleSourceRecordedName(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(map[bool]string{false: "unchanged", true: "changed"}[changed], func(t *testing.T) {
			repo, commit := upstreamRepo(t)
			source := sourceCase(t, repo, commit)
			run, opts := oracleReading(t)
			caseDir := filepath.Join(opts.BenchDir, "cases", filepath.Base(source))
			for _, name := range []string{KeyFile, SourceFile, "fix.patch"} {
				data, err := os.ReadFile(filepath.Join(source, name))
				if err != nil {
					t.Fatal(err)
				}
				writeFile(t, caseDir, name, string(data))
			}
			opts.CaseCache = t.TempDir()
			materialized, err := MaterializeCase(caseDir, opts.CaseCache)
			if err != nil {
				t.Fatal(err)
			}
			recorded := filepath.Base(materialized)
			ws := filepath.Join(run, recorded, "1", "ws")
			writeFile(t, ws, "agent_test.go", "package calc\nimport \"testing\"\nfunc TestAdd(t *testing.T) {}\n")
			data, err := json.Marshal(Aggregate{Cases: []Result{{Case: recorded, Run: 1, Workspace: ws, Catch: CatchResult{TestFiles: []string{"agent_test.go"}}}}})
			if err != nil {
				t.Fatal(err)
			}
			writeFile(t, run, "aggregate.json", string(data))
			if changed {
				writeFile(t, caseDir, "fix.patch", strings.ReplaceAll(sourcePatch, "a + b", "b + a"))
			}
			calls := 0
			opts.Agent = func(context.Context, string, Key, Options) (AgentResult, error) {
				calls++
				return AgentResult{Result: `{"tests":[{"name":"TestAdd","verdict":"contract","reason":"documented"}]}`}, nil
			}
			report, err := VerifyOracle(run, opts)
			if changed {
				if err == nil || !strings.Contains(err.Error(), recorded) || !strings.Contains(err.Error(), "changed since the reading") {
					t.Fatalf("err=%v, want case changed since the reading", err)
				}
				if calls != 0 || len(report.Files) != 0 {
					t.Fatalf("judged changed fixture: calls=%d report=%+v", calls, report)
				}
				if _, err := os.Stat(filepath.Join(run, "oracle", "verdicts.json")); !os.IsNotExist(err) {
					t.Fatalf("verdicts exist: %v", err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if calls != 1 || report.Verdicts["contract"] != 1 {
					t.Fatalf("calls=%d report=%+v", calls, report)
				}
			}
		})
	}
}
