package bench

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func oracleReading(t *testing.T) (string, Options) {
	t.Helper()
	root := t.TempDir()
	put := func(p, s string) {
		t.Helper()
		p = filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(s), 0644); err != nil {
			t.Fatal(err)
		}
	}
	put("bench/cases/demo/fixture/README.md", "# Contract\nAnswer returns forty-two.\n")
	put("bench/cases/demo/fixture/go.mod", "module example.com/demo\n\ngo 1.22\n")
	put("bench/cases/demo/fixture/api.go", "package demo\n// Answer returns forty-two.\nfunc Answer() int { /* SECRET_BODY_MARKER */ return 42 }\n")
	put("bench/cases/demo/KEY.json", `{"request":"Test Answer","defects":[{"id":"SECRET_DEFECT"}]}`)
	put("bench/cases/demo/fix/all/secret.txt", "SECRET_FIX")
	put("reading/demo/1/ws/agent_test.go", "package demo\nimport \"testing\"\nfunc TestContract(t *testing.T) {}\nfunc TestInvented(t *testing.T) {}\n")
	r := Result{Case: "demo", Run: 1, Workspace: filepath.Join(root, "reading/demo/1/ws"), Catch: CatchResult{Checked: true, TestFiles: []string{"agent_test.go"}, Caught: map[string]bool{"D1": true}, CaughtBy: map[string][]string{"D1": {"TestContract"}}, InvertedBy: map[string][]string{"D2": {"TestInvented"}}, BrokenTests: 1}}
	b, _ := json.Marshal(Aggregate{Cases: []Result{r}})
	put("reading/aggregate.json", string(b))
	put("reading/demo/1/result.json", `{"original":true}`)
	return filepath.Join(root, "reading"), Options{BenchDir: filepath.Join(root, "bench")}
}

func TestOracleBlindInputAndRecomputedReading(t *testing.T) {
	for _, malformed := range []bool{false, true} {
		t.Run(map[bool]string{false: "verdicts", true: "unparseable-keeps-tests"}[malformed], func(t *testing.T) {
			run, opts := oracleReading(t)
			original, _ := os.ReadFile(filepath.Join(run, "aggregate.json"))
			result, _ := os.ReadFile(filepath.Join(run, "demo/1/result.json"))
			calls := 0
			opts.Agent = func(_ context.Context, ws string, key Key, o Options) (AgentResult, error) {
				calls++
				if len(key.Defects) != 0 || key.Request != nil {
					t.Fatal("answer key passed to verifier")
				}
				var all strings.Builder
				err := filepath.Walk(ws, func(p string, info os.FileInfo, err error) error {
					if err != nil {
						return err
					}
					if !info.IsDir() {
						b, e := os.ReadFile(p)
						if e != nil {
							return e
						}
						all.Write(b)
					}
					return nil
				})
				if err != nil {
					t.Fatal(err)
				}
				s := all.String()
				for _, want := range []string{"# Contract", "func Answer() int", "Answer returns forty-two", "TestContract", "Test Answer"} {
					if !strings.Contains(s, want) {
						t.Errorf("missing blind input %q", want)
					}
				}
				for _, bad := range []string{"SECRET_BODY_MARKER", "SECRET_DEFECT", "SECRET_FIX"} {
					if strings.Contains(s, bad) {
						t.Errorf("leaked %s", bad)
					}
				}
				if !strings.Contains(o.Prompt, "invented") {
					t.Fatal("missing judging prompt")
				}
				answer := `{"tests":[{"name":"TestContract","verdict":"contract","reason":"documented"},{"name":"TestInvented","verdict":"invented","reason":"not documented"}]}`
				if malformed {
					answer = "not JSON"
				}
				return AgentResult{Result: answer, CostUSD: 0.25, Turns: 2}, nil
			}
			report, err := VerifyOracle(run, opts)
			if err != nil {
				t.Fatal(err)
			}
			if calls != 1 {
				t.Fatalf("calls=%d", calls)
			}
			if report.Recorded != (OracleCounts{Caught: 1, Broken: 1}) {
				t.Fatalf("recorded=%+v", report.Recorded)
			}
			want := OracleCounts{Caught: 1}
			if malformed {
				want.Broken = 1
			}
			if report.Filtered != want {
				t.Fatalf("filtered=%+v want %+v", report.Filtered, want)
			}
			if report.CostUSD != .25 || report.Turns != 2 {
				t.Fatalf("cost/turns=%+v", report)
			}
			if malformed && report.Verdicts["unclear"] != 2 {
				t.Fatalf("verdicts=%v", report.Verdicts)
			}
			stored, err := os.ReadFile(filepath.Join(run, "oracle", "verdicts.json"))
			if err != nil {
				t.Fatal(err)
			}
			var persisted OracleReport
			if err := json.Unmarshal(stored, &persisted); err != nil {
				t.Fatal(err)
			}
			if persisted.Recorded != report.Recorded || persisted.Filtered != want || persisted.PromptDigest == "" || persisted.Runner == "" || persisted.Model == "" {
				t.Fatalf("stored report=%+v", persisted)
			}
			summary, err := os.ReadFile(filepath.Join(run, "oracle", "summary.md"))
			if err != nil {
				t.Fatal(err)
			}
			row := "| Dropping invented | 1 | 0 | 0 |"
			if malformed {
				row = "| Dropping invented | 1 | 1 | 0 |"
			}
			if !strings.Contains(string(summary), "| As recorded | 1 | 1 | 0 |") || !strings.Contains(string(summary), row) {
				t.Fatalf("summary=%s", summary)
			}
			for _, name := range []string{"verdicts.json", "summary.md"} {
				b, e := os.ReadFile(filepath.Join(run, "oracle", name))
				if e != nil || len(b) == 0 {
					t.Fatalf("%s: %s %v", name, b, e)
				}
			}
			after, _ := os.ReadFile(filepath.Join(run, "aggregate.json"))
			if string(after) != string(original) {
				t.Fatal("aggregate changed")
			}
			after, _ = os.ReadFile(filepath.Join(run, "demo/1/result.json"))
			if string(after) != string(result) {
				t.Fatal("result changed")
			}
		})
	}
}

func TestOracleStrictVerdictSchema(t *testing.T) {
	for _, answer := range []string{
		`{"tests":[{"name":"TestA","verdict":"contract","reason":"yes"}],"extra":true}`,
		`{"tests":[{"name":"TestA","verdict":"contract","reason":"yes"}]} trailing`,
		`{"tests":[{"name":"TestA","verdict":"maybe","reason":"yes"}]}`,
		`{"tests":[{"name":"TestA","verdict":"contract","reason":""}]}`,
		`{"tests":[]}`,
		`{"tests":[{"name":"TestA","verdict":"invented","verdict":"contract","reason":"yes"}]}`,
		`{"tests":[{"name":"TestA","verdict":"contract","reason":"yes"}`,
	} {
		if _, err := oracleParse(answer, []string{"TestA"}); err == nil {
			t.Errorf("accepted %s", answer)
		}
	}
}

func TestOracleCanonicalWitnessNamesAndNoWitness(t *testing.T) {
	c := CatchResult{Caught: map[string]bool{"D1": true}, CaughtBy: map[string][]string{"D1": {"example.com/demo.TestContract/sub"}}}
	names := oracleNames("agent_test.go", []byte("package demo; import \"testing\"; func TestContract(t *testing.T) {}"), c)
	if len(names) != 1 || names[0] != "example.com/demo.TestContract/sub" {
		t.Fatalf("names=%v", names)
	}
	before, after, _ := oracleRecount(c, map[string]bool{names[0]: true})
	if before.Caught != 1 || before.NoWitness != 0 || after.Caught != 0 || after.NoWitness != 1 {
		t.Fatalf("before=%+v after=%+v", before, after)
	}
}

func TestOracleRefusesWithoutWriting(t *testing.T) {
	for _, kind := range []string{"missing-run", "not-reading", "no-workspace"} {
		t.Run(kind, func(t *testing.T) {
			run, opts := oracleReading(t)
			switch kind {
			case "missing-run":
				run = ""
			case "not-reading":
				run = t.TempDir()
			case "no-workspace":
				b, _ := json.Marshal(Aggregate{Cases: []Result{{Case: "without-kept-ws"}}})
				if err := os.WriteFile(filepath.Join(run, "aggregate.json"), b, 0644); err != nil {
					t.Fatal(err)
				}
			}
			opts.Agent = func(context.Context, string, Key, Options) (AgentResult, error) {
				t.Fatal("spawned on refusal")
				return AgentResult{}, nil
			}
			_, err := VerifyOracle(run, opts)
			if err == nil {
				t.Fatal("accepted invalid reading")
			}
			if kind == "no-workspace" && !strings.Contains(err.Error(), "rerun with --keep") {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(run, "oracle")); !os.IsNotExist(err) {
				t.Fatal("wrote on refusal")
			}
		})
	}
}
