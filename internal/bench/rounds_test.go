package bench

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

// witnessName is witnessTest's test as RedCheck names it: the package path of escalateCase's model package.
const witnessName = "example.com/sub/model.TestOrderIsSorted"

// roundStep is what one agent call leaves in the workspace: a test file, or nothing when file is empty.
type roundStep struct {
	file, content string
}

// roundsAgent plays one scripted call per round: call i writes steps[i], and the agent records the follow-up it was
// told and the test files the workspace already held when it started.
type roundsAgent struct {
	mu        sync.Mutex
	steps     []roundStep
	fail      map[int]bool
	followups []string
	held      [][]string
}

func (a *roundsAgent) run(ctx context.Context, ws string, key Key, opts Options) (AgentResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	call := len(a.followups)
	a.followups = append(a.followups, opts.Followup)
	paths, _ := filepath.Glob(filepath.Join(ws, "model", "*_test.go"))
	held := []string{}
	for _, p := range paths {
		held = append(held, filepath.Base(p))
	}
	a.held = append(a.held, held)
	if a.fail[call] {
		return AgentResult{IsError: true, ErrorText: "provider down", CostUSD: 1, Turns: 1}, nil
	}
	if call < len(a.steps) && a.steps[call].file != "" {
		if err := os.WriteFile(filepath.Join(ws, "model", a.steps[call].file), []byte(a.steps[call].content), 0o644); err != nil {
			return AgentResult{}, err
		}
	}
	return AgentResult{CostUSD: 1, Turns: 3}, nil
}

func runRounds(t *testing.T, rounds int, agent *roundsAgent) Aggregate {
	t.Helper()
	if testing.Short() {
		t.Skip("runs git and go test")
	}
	for _, bin := range []string{"go", "git"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not on PATH", bin)
		}
	}
	agg, _ := Run(Options{CasesGlob: escalateCase(t), Runs: 1, Model: "m", Rounds: rounds,
		Timeout: 2 * time.Minute, SuiteTimeout: 2 * time.Minute, Out: filepath.Join(t.TempDir(), "results"),
		BenchDir: t.TempDir(), Agent: agent.run})
	if len(agg.Cases) != 1 {
		t.Fatalf("cases = %d, want 1", len(agg.Cases))
	}
	return agg
}

// withoutSeconds drops the wall clock, the one per-round field a test cannot predict.
func withoutSeconds(rounds []RoundResult) []RoundResult {
	out := append([]RoundResult(nil), rounds...)
	for i := range out {
		out[i].Seconds = 0
	}
	return out
}

func holds(files []string, name string) bool {
	for _, f := range files {
		if f == name {
			return true
		}
	}
	return false
}

// A later round works in the same workspace and is told which tests earlier rounds left failing on the unmodified
// code, so it can look for other defects instead of pinning the same one again. Each round's record says what the
// workspace held once it ended, and the run pays for every round.
func TestRoundsTellALaterRoundTheTestsEarlierRoundsLeftFailing(t *testing.T) {
	agent := &roundsAgent{steps: []roundStep{{"agent_test.go", witnessTest}, {}}}
	agg := runRounds(t, 2, agent)
	res := agg.Cases[0]
	if len(agent.followups) != 2 {
		t.Fatalf("the agent ran %d times, want one call per round (2); notes %v", len(agent.followups), res.Notes)
	}
	if agent.followups[0] != "" {
		t.Errorf("the first round was given a follow-up: %q", agent.followups[0])
	}
	if !strings.Contains(agent.followups[1], "TestOrderIsSorted") {
		t.Errorf("the second round was not told the test the first left failing: %q", agent.followups[1])
	}
	if !holds(agent.held[1], "agent_test.go") {
		t.Errorf("the second round started without the first round's test (held %v): rounds share the workspace", agent.held[1])
	}
	// RedCheck names a Go test by its package path, as `tsp redcheck` prints it.
	want := []RoundResult{
		{Round: 1, Witnesses: []string{witnessName}, NewWitnesses: 1, Caught: 1, Turns: 3, CostUSD: 1},
		{Round: 2, Witnesses: []string{witnessName}, NewWitnesses: 0, Caught: 1, Turns: 3, CostUSD: 1},
	}
	if got := withoutSeconds(res.Rounds); !reflect.DeepEqual(got, want) {
		t.Errorf("rounds = %+v\nwant     %+v", got, want)
	}
	if res.Caught != 1 || res.CostUSD != 2 || res.Turns != 6 {
		t.Errorf("caught = %d cost = %v turns = %d, want 1 caught and both rounds paid (2, 6)", res.Caught, res.CostUSD, res.Turns)
	}
}

// A round that pins nothing tells the next round that nothing is pinned yet, and a defect a later round pins is
// counted at that round, in the run and in the reading's per-round totals, which also name the rounds in the
// provenance so the reading is never compared with a one-pass one.
func TestRoundsRecordWhereALaterRoundCaughtTheDefect(t *testing.T) {
	agent := &roundsAgent{steps: []roundStep{{"agent_test.go", blindTest}, {"agent2_test.go", witnessTest}}}
	agg := runRounds(t, 2, agent)
	res := agg.Cases[0]
	if len(agent.followups) != 2 {
		t.Fatalf("the agent ran %d times, want 2; notes %v", len(agent.followups), res.Notes)
	}
	if f := agent.followups[1]; f == "" || strings.Contains(f, "TestOrderHasThreeLetters") || !strings.Contains(f, "no defect is pinned yet") {
		t.Errorf("after a round that pinned nothing the follow-up must say nothing is pinned and name no test: %q", f)
	}
	want := []RoundResult{
		{Round: 1, Witnesses: []string{}, NewWitnesses: 0, Caught: 0, Turns: 3, CostUSD: 1},
		{Round: 2, Witnesses: []string{witnessName}, NewWitnesses: 1, Caught: 1, Turns: 3, CostUSD: 1},
	}
	if got := withoutSeconds(res.Rounds); !reflect.DeepEqual(got, want) {
		t.Errorf("rounds = %+v\nwant     %+v", got, want)
	}
	if res.Caught != 1 {
		t.Errorf("caught = %d, want the defect the second round pinned", res.Caught)
	}
	if !reflect.DeepEqual(agg.CaughtByRound, []int{0, 1}) || !reflect.DeepEqual(agg.BrokenByRound, []int{0, 0}) {
		t.Errorf("caught by round = %v broken by round = %v, want [0 1] and [0 0]", agg.CaughtByRound, agg.BrokenByRound)
	}
	if agg.Provenance.Rounds != 2 {
		t.Errorf("provenance rounds = %d, want 2", agg.Provenance.Rounds)
	}
	if summary := Summary(agg); !strings.Contains(summary, "caught after each round (defect-runs): 0, 1") {
		t.Errorf("the summary does not show the per-round catches:\n%s", summary)
	}
}

// Without rounds a run is the single pass it always was: one call, no follow-up, and no rounds in the result, the
// aggregate or the provenance.
func TestWithoutRoundsARunIsOnePass(t *testing.T) {
	for _, rounds := range []int{0, 1} {
		agent := &roundsAgent{steps: []roundStep{{"agent_test.go", witnessTest}}}
		agg := runRounds(t, rounds, agent)
		if len(agent.followups) != 1 || agent.followups[0] != "" {
			t.Errorf("rounds %d: calls %d follow-ups %q, want one call with none", rounds, len(agent.followups), agent.followups)
		}
		if agg.Cases[0].Rounds != nil || agg.CaughtByRound != nil || agg.BrokenByRound != nil || agg.Provenance.Rounds != 0 {
			t.Errorf("rounds %d: a one-pass reading records rounds: result %v caught %v broken %v provenance %d",
				rounds, agg.Cases[0].Rounds, agg.CaughtByRound, agg.BrokenByRound, agg.Provenance.Rounds)
		}
		if strings.Contains(Summary(agg), "after each round") {
			t.Errorf("rounds %d: a one-pass summary shows per-round totals", rounds)
		}
	}
}

// A later round that still fails after its retries fails the run, as an escalated attempt does: a run that did not
// make its rounds has no verdict to add to the reading.
func TestALaterRoundThatFailsFailsTheRun(t *testing.T) {
	agent := &roundsAgent{steps: []roundStep{{"agent_test.go", witnessTest}}, fail: map[int]bool{1: true}}
	agg := runRounds(t, 2, agent)
	res := agg.Cases[0]
	if !res.Failed || agg.Failed != 1 || agg.Caught != 0 {
		t.Fatalf("failed = %v (aggregate %d) caught = %d, want a failed run outside the catch count; reason %q", res.Failed, agg.Failed, agg.Caught, res.FailReason)
	}
	if !strings.Contains(res.FailReason, "provider down") {
		t.Errorf("fail reason %q does not carry the round's error", res.FailReason)
	}
}
