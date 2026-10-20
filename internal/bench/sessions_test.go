package bench

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func sessionCase(t *testing.T) string {
	t.Helper()
	dir := escalateCase(t)
	key, err := os.ReadFile(filepath.Join(dir, KeyFile))
	if err != nil {
		t.Fatal(err)
	}
	key = []byte(strings.Replace(string(key), `"defects":[`, `"defects":[{"id":"D2","file":"model/model.go","line":4,"class":"value","keywords":["value"],"description":"x","trigger":{"input":"i","expected":"e","actual":"a"},"why_missed":"w"},`, 1))
	if err := os.WriteFile(filepath.Join(dir, KeyFile), key, 0644); err != nil {
		t.Fatal(err)
	}
	versions := map[string]string{"fixture": "cab", "fix/all": "abc", "fix/keep-D1": "cab", "fix/keep-D2": "abc"}
	for version, order := range versions {
		path := filepath.Join(dir, version, "model")
		if err := os.MkdirAll(path, 0755); err != nil {
			t.Fatal(err)
		}
		value := 1
		if version == "fixture" || version == "fix/keep-D2" {
			value = 0
		}
		source := "package model\nfunc Order() string { return \"" + order + "\" }\nfunc Value() int { return " + string(rune('0'+value)) + " }\n"
		if err := os.WriteFile(filepath.Join(path, "model.go"), []byte(source), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestSessionsOnePassKeepsTheOriginalRunShape(t *testing.T) {
	if testing.Short() {
		t.Skip("runs git and go test")
	}
	dir := sessionCase(t)
	for _, n := range []int{0, 1} {
		calls := 0
		agent := func(ctx context.Context, ws string, key Key, opts Options) (AgentResult, error) {
			calls++
			return AgentResult{CostUSD: 1, Turns: 3}, nil
		}
		agg, code := Run(Options{CasesGlob: dir, Sessions: n, Runs: 1, Timeout: time.Minute, SuiteTimeout: time.Minute, Out: filepath.Join(t.TempDir(), "results"), Agent: agent})
		if code != 0 || calls != 1 || agg.Cases[0].Sessions != nil || agg.CaughtBySession != nil || agg.Provenance.Sessions != 0 || strings.Contains(Summary(agg), "per session") {
			t.Fatalf("sessions %d: code %d calls %d aggregate %+v", n, code, calls, agg)
		}
	}
}

func TestSessionsFailureKeepsWorkButDoesNotEnterAggregate(t *testing.T) {
	if testing.Short() {
		t.Skip("runs git and go test")
	}
	calls := 0
	agent := func(ctx context.Context, ws string, key Key, opts Options) (AgentResult, error) {
		calls++
		if calls == 1 {
			return AgentResult{CostUSD: 1, Turns: 2}, nil
		}
		return AgentResult{CostUSD: 1, Turns: 2, IsError: true, ErrorText: "provider down"}, nil
	}
	agg, code := Run(Options{CasesGlob: sessionCase(t), Sessions: 3, Runs: 1, Retries: 1, Timeout: time.Minute, SuiteTimeout: time.Minute, Out: filepath.Join(t.TempDir(), "results"), Agent: agent})
	if code != ExitPartial || calls != 3 || agg.Failed != 1 {
		t.Fatalf("code %d calls %d failed %d", code, calls, agg.Failed)
	}
	r := agg.Cases[0]
	if len(r.Sessions) != 2 || r.Sessions[1].Turns != 4 || r.Sessions[1].CostUSD != 2 || !strings.Contains(r.Sessions[1].StopReason, "provider down") || len(agg.CaughtBySession) != 0 {
		t.Fatalf("result %+v aggregate %+v", r, agg)
	}
}

func TestSessionFixCannotRepresentMultipleRemainingDefects(t *testing.T) {
	ws := t.TempDir()
	err := applySessionFix(t.TempDir(), ws, []string{"D1", "D2"})
	if err == nil || !strings.Contains(err.Error(), "remaining defects [D1 D2]") {
		t.Fatalf("error %v", err)
	}
}

func TestSessionsOnePassAndComparisonCompatibility(t *testing.T) {
	for _, n := range []int{0, 1} {
		if err := refuseSessions(Provenance{}, Provenance{Sessions: n}); err != nil {
			t.Fatal(err)
		}
	}
	if err := refuseSessions(Provenance{}, Provenance{Sessions: 2}); err == nil {
		t.Fatal("mixed sessions accepted")
	}
	a := Aggregate{Cases: []Result{{Sessions: []SessionResult{{Caught: []string{"D1"}, Remaining: []string{"D2"}}, {Caught: []string{"D2"}, Remaining: []string{}}}}}}
	finalizeAggregate(&a)
	finalizeAggregate(&a)
	if !reflect.DeepEqual(a.CaughtBySession, []int{1, 1}) || !reflect.DeepEqual(a.CompletedBySession, []int{0, 1}) {
		t.Fatalf("aggregate %+v", a)
	}
	if !strings.Contains(Summary(a), "new catches per session") {
		t.Fatal(Summary(a))
	}
}

func TestSessionsApplyOnlyCaughtFixesAndKeepTests(t *testing.T) {
	if testing.Short() {
		t.Skip("runs git and go test")
	}
	for _, nothing := range []bool{false, true} {
		t.Run(map[bool]string{false: "fixes", true: "no catch"}[nothing], func(t *testing.T) {
			dir := sessionCase(t)
			calls := 0
			agent := func(ctx context.Context, ws string, key Key, opts Options) (AgentResult, error) {
				calls++
				if got := agentPrompt(key, opts); got != agentPrompt(key, Options{}) {
					t.Errorf("session prompt = %q", got)
				}
				if calls == 2 {
					source, _ := os.ReadFile(filepath.Join(ws, "model/model.go"))
					want, _ := os.ReadFile(filepath.Join(dir, "fix/keep-D2/model/model.go"))
					if nothing {
						want, _ = os.ReadFile(filepath.Join(dir, "fixture/model/model.go"))
					}
					if string(source) != string(want) {
						t.Errorf("second session source = %s, want %s", source, want)
					}
					if !nothing {
						if _, err := os.Stat(filepath.Join(ws, "model/agent_test.go")); err != nil {
							t.Error(err)
						}
					}
				}
				if !nothing {
					name, content := "agent_test.go", witnessTest
					if calls > 1 {
						name, content = "value_test.go", "package model\nimport \"testing\"\nfunc TestValue(t *testing.T) { if Value()!=1 { t.Fatal(Value()) } }\n"
					}
					if err := os.WriteFile(filepath.Join(ws, "model", name), []byte(content), 0644); err != nil {
						return AgentResult{}, err
					}
				}
				return AgentResult{CostUSD: 1, Turns: 3}, nil
			}
			n := 3
			if nothing {
				n = 2
			}
			agg, code := Run(Options{CasesGlob: dir, Runs: 1, Model: "m", Sessions: n, Timeout: time.Minute, SuiteTimeout: time.Minute, Out: filepath.Join(t.TempDir(), "results"), Agent: agent, Keep: true})
			if code != 0 || len(agg.Cases) != 1 {
				t.Fatalf("code %d cases %+v", code, agg.Cases)
			}
			if calls != 2 {
				t.Fatalf("calls %d, want 2", calls)
			}
			if nothing {
				if !reflect.DeepEqual(agg.CaughtBySession, []int{0, 0}) {
					t.Fatal(agg.CaughtBySession)
				}
				return
			}
			res := agg.Cases[0]
			if len(res.Sessions) != 2 || !reflect.DeepEqual(res.Sessions[0].Remaining, []string{"D2"}) || len(res.Sessions[1].Remaining) != 0 {
				t.Fatalf("sessions %+v", res.Sessions)
			}
			if !reflect.DeepEqual(agg.CaughtBySession, []int{1, 1}) || agg.Caught != 2 || agg.CompletedBySession[1] != 1 || agg.Provenance.Sessions != 3 {
				t.Fatalf("aggregate %+v", agg)
			}
			if res.Turns != 6 || res.CostUSD != 2 {
				t.Fatalf("work %+v", res)
			}
		})
	}
}
