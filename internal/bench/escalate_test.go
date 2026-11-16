package bench

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/alesierraalta/tsp/internal/feature"
	"github.com/alesierraalta/tsp/internal/state"
)

// escalateCase is a Go case whose only defect sits in model.Order ("cab" where "abc" is promised).
func escalateCase(t *testing.T) string {
	t.Helper()
	caseDir, _ := goSubpackageCase(t, "go test ./...")
	key := `{"id":"esc","language":"go","suite":"go test ./...","surface":"lib","defects":[{"id":"D1","file":"model/model.go","line":3,"class":"order","keywords":["order"],"description":"x","trigger":{"input":"i","expected":"e","actual":"a"},"why_missed":"w"}]}`
	if err := os.WriteFile(filepath.Join(caseDir, KeyFile), []byte(key), 0o644); err != nil {
		t.Fatal(err)
	}
	return caseDir
}

const (
	blindTest   = "package model\n\nimport \"testing\"\n\nfunc TestOrderHasThreeLetters(t *testing.T) {\n\tif len(Order()) != 3 {\n\t\tt.Fatal(Order())\n\t}\n}\n"
	witnessTest = "package model\n\nimport \"testing\"\n\nfunc TestOrderIsSorted(t *testing.T) {\n\tif Order() != \"abc\" {\n\t\tt.Fatal(Order())\n\t}\n}\n"
)

// modelAgent writes the test each model would leave and records which models ran, and whether the workspace it
// got still held a test an earlier model wrote.
type modelAgent struct {
	mu       sync.Mutex
	models   []string
	inherits []bool
	tests    map[string]string
}

func (m *modelAgent) run(ctx context.Context, ws string, key Key, opts Options) (AgentResult, error) {
	m.mu.Lock()
	m.models = append(m.models, opts.Model)
	_, err := os.Stat(filepath.Join(ws, "model", "agent_test.go"))
	m.inherits = append(m.inherits, err == nil)
	m.mu.Unlock()
	if err := os.WriteFile(filepath.Join(ws, "model", "agent_test.go"), []byte(m.tests[opts.Model]), 0o644); err != nil {
		return AgentResult{}, err
	}
	cost := map[string]float64{"cheap": 1, "strong": 2}[opts.Model]
	return AgentResult{CostUSD: cost, Turns: 3}, nil
}

func runEscalation(t *testing.T, tests map[string]string, escalateModel string) (Result, *modelAgent) {
	t.Helper()
	agg, agent := runEscalationAggregate(t, tests, escalateModel)
	return agg.Cases[0], agent
}

func runEscalationAggregate(t *testing.T, tests map[string]string, escalateModel string) (Aggregate, *modelAgent) {
	t.Helper()
	if testing.Short() {
		t.Skip("runs git and go test")
	}
	for _, bin := range []string{"go", "git"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not on PATH", bin)
		}
	}
	agent := &modelAgent{tests: tests}
	agg, _ := Run(Options{CasesGlob: escalateCase(t), Runs: 1, Model: "cheap", EscalateModel: escalateModel,
		Timeout: 2 * time.Minute, SuiteTimeout: 2 * time.Minute, Out: filepath.Join(t.TempDir(), "results"),
		BenchDir: t.TempDir(), Agent: agent.run})
	if len(agg.Cases) != 1 {
		t.Fatalf("cases = %d, want 1", len(agg.Cases))
	}
	return agg, agent
}

// A reading that escalated says so: it counts the escalated runs and names the escalation model in its
// provenance, so it is never compared with a plain reading as if both ran one model.
func TestRunRecordsEscalationInTheAggregate(t *testing.T) {
	agg, _ := runEscalationAggregate(t, map[string]string{"cheap": blindTest, "strong": witnessTest}, "strong")
	if agg.Escalated != 1 {
		t.Errorf("escalated = %d, want 1", agg.Escalated)
	}
	if agg.Provenance.EscalateModel != "strong" {
		t.Errorf("provenance escalate_model = %q, want strong", agg.Provenance.EscalateModel)
	}
	if p := provenanceForRun(Options{}, "c", 1); p.EscalateModel != "" {
		t.Errorf("a plain reading names an escalation model: %q", p.EscalateModel)
	}
}

// A cheap run that leaves no test failing on the unmodified code has pinned nothing, so the case runs again with
// the escalation model in a fresh workspace, and the run is scored on that second attempt and pays for both.
func TestRunEscalatesARunThatPinsNothing(t *testing.T) {
	res, agent := runEscalation(t, map[string]string{"cheap": blindTest, "strong": witnessTest}, "strong")
	if want := []string{"cheap", "strong"}; !reflect.DeepEqual(agent.models, want) {
		t.Fatalf("models = %v, want %v", agent.models, want)
	}
	if agent.inherits[1] {
		t.Error("the escalated attempt saw the cheap model's test: it must start from the fixture")
	}
	if !res.Escalated || res.Caught != 1 {
		t.Fatalf("escalated = %v caught = %d, want an escalated run that catches D1; notes %v", res.Escalated, res.Caught, res.Notes)
	}
	if res.CostUSD != 3 || res.Turns != 6 {
		t.Errorf("cost = %v turns = %d, want both attempts counted (3, 6)", res.CostUSD, res.Turns)
	}
}

// A cheap run that already pinned a defect is kept: escalating it would pay the strong model for nothing.
func TestRunKeepsACheapRunThatPinsADefect(t *testing.T) {
	res, agent := runEscalation(t, map[string]string{"cheap": witnessTest, "strong": witnessTest}, "strong")
	if want := []string{"cheap"}; !reflect.DeepEqual(agent.models, want) {
		t.Fatalf("models = %v, want %v", agent.models, want)
	}
	if res.Escalated || res.Caught != 1 || res.CostUSD != 1 {
		t.Fatalf("escalated = %v caught = %d cost = %v, want the cheap run kept", res.Escalated, res.Caught, res.CostUSD)
	}
}

// Without an escalation model a run is scored as it ran, whatever it pinned.
func TestRunWithoutAnEscalationModelRunsOnce(t *testing.T) {
	res, agent := runEscalation(t, map[string]string{"cheap": blindTest}, "")
	if want := []string{"cheap"}; !reflect.DeepEqual(agent.models, want) {
		t.Fatalf("models = %v, want %v", agent.models, want)
	}
	if res.Escalated || res.Caught != 0 {
		t.Fatalf("escalated = %v caught = %d, want one unescalated miss", res.Escalated, res.Caught)
	}
}

// productLoopEnabledAt answers whether the run's config copy carries the opt-in headless loop, read the way the
// extension's own `tsp status --json` reads it (internal/state over the copy's own state file).
func productLoopStateAt(agentDir string) string {
	b, err := os.ReadFile(filepath.Join(agentDir, IsolatedHomeDir, ".config", "tsp", "state.json"))
	if err != nil {
		return "missing: " + err.Error()
	}
	var st state.State
	if err := json.Unmarshal(b, &st); err != nil {
		return "unreadable: " + err.Error()
	}
	if st.Features[feature.HeadlessLoopID].Enabled {
		return "on"
	}
	return "off"
}

// loopRecordingAgent records, per attempt, whether the config copy it was handed had the headless loop on.
type loopRecordingAgent struct {
	mu    sync.Mutex
	tests map[string]string
	seen  []string
}

func (a *loopRecordingAgent) run(ctx context.Context, ws string, key Key, opts Options) (AgentResult, error) {
	a.mu.Lock()
	a.seen = append(a.seen, productLoopStateAt(opts.ConfigDir))
	a.mu.Unlock()
	if err := os.WriteFile(filepath.Join(ws, "model", "agent_test.go"), []byte(a.tests[opts.Model]), 0o644); err != nil {
		return AgentResult{}, err
	}
	return AgentResult{CostUSD: 1, Turns: 2}, nil
}

// An escalation rescaffolds the workspace and re-clones the config, so the loop toggle has to ride every clone: a
// reading that says headless_loop must never be one whose escalated attempt ran without it.
func TestAnEscalatedProductRunStillHasTheHeadlessLoopEnabled(t *testing.T) {
	if testing.Short() {
		t.Skip("runs git and go test")
	}
	for _, bin := range []string{"go", "git"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not on PATH", bin)
		}
	}
	cfg := filepath.Join(t.TempDir(), "product")
	if err := os.MkdirAll(filepath.Join(cfg, IsolatedHomeDir), 0o700); err != nil {
		t.Fatal(err)
	}
	agent := &loopRecordingAgent{tests: map[string]string{"cheap": blindTest, "strong": witnessTest}}
	Run(Options{CasesGlob: escalateCase(t), Runs: 1, Model: "cheap", EscalateModel: "strong",
		ConfigMode: ConfigProduct, ConfigDir: cfg, Timeout: 2 * time.Minute, SuiteTimeout: 2 * time.Minute,
		Out: filepath.Join(t.TempDir(), "results"), BenchDir: t.TempDir(), Agent: agent.run})
	if len(agent.seen) != 2 {
		t.Fatalf("attempts = %v, want 2 (the escalation runs a second one)", agent.seen)
	}
	for i, state := range agent.seen {
		if state != "on" {
			t.Fatalf("attempt %d was handed a config copy whose headless loop is %q, while the reading still claims headless_loop", i+1, state)
		}
	}
}
