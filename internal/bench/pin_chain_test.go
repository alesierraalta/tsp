package bench

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	plancheck "github.com/alesierraalta/tsp/internal/plan"
)

// chainWS is a workspace whose plan holds the given Pinning rows. A blank body is a workspace with no plan at all.
func chainWS(t *testing.T, body string) string {
	t.Helper()
	ws := t.TempDir()
	if body == "" {
		return ws
	}
	plan := "# Test plan\n\n## Findings\n\n" + pinnedHeader + "\n## Pinning\n\n" + plancheck.PinningHeader + body
	writeFile(t, ws, PlanPath, plan)
	return ws
}

// The chain arm is measured from the plan it wrote: one pinning row per finding, and the status says what the run
// did with it. A plan or a section that is not there recorded nothing, which is a zero, not an error.
func TestScorePinningCountsWhatTheChainDidWithEachFinding(t *testing.T) {
	t.Run("one pinned, one blocked and one open count 1/1/1", func(t *testing.T) {
		ws := chainWS(t, ""+
			"| P1 | src/a.mjs:1 | add returns a wrong sum | pinned | tests/a.test.mjs :: adds | - |\n"+
			"| P2 | src/b.mjs:40 | no suite can reach it | blocked | - | the module cannot be imported |\n"+
			"| P3 | src/c.mjs:7 | input is not validated | open | - | - |\n")
		got := ScorePinning(ws)
		if got.Pinned != 1 || got.Blocked != 1 || got.Open != 1 || got.Refuted != 0 {
			t.Fatalf("counts = %d pinned, %d blocked, %d open, %d refuted; want 1/1/1/0", got.Pinned, got.Blocked, got.Open, got.Refuted)
		}
		if len(got.Rows) != 3 {
			t.Fatalf("%d rows, want 3: %+v", len(got.Rows), got.Rows)
		}
		first := got.Rows[0]
		if first.ID != "P1" || first.Location != "src/a.mjs:1" || first.Claim != "add returns a wrong sum" ||
			first.Status != pinStatusPinned || first.Test != "tests/a.test.mjs :: adds" {
			t.Fatalf("row = %+v, want the finding as the review worded it and the test that pins it", first)
		}
	})

	t.Run("a refuted finding is the rejected row tsp pin --refute writes", func(t *testing.T) {
		ws := chainWS(t, "| P1 | src/a.mjs:1 | the claim does not hold | rejected | - | the function never sees that input |\n")
		got := ScorePinning(ws)
		if got.Refuted != 1 || got.Pinned != 0 || got.Blocked != 0 || got.Open != 0 {
			t.Fatalf("counts = %d refuted, %d pinned; want 1/0", got.Refuted, got.Pinned)
		}
	})

	for _, tc := range []struct {
		name, body string
		plan       bool
	}{
		{name: "no plan at all", body: ""},
		{name: "a plan with no Pinning section", body: "", plan: true},
	} {
		t.Run(tc.name+" recorded nothing", func(t *testing.T) {
			ws := chainWS(t, "")
			if tc.plan {
				writeFile(t, ws, PlanPath, "# Test plan\n\n## Findings\n\n"+pinnedHeader)
			}
			got := ScorePinning(ws)
			if got.Pinned != 0 || got.Refuted != 0 || got.Blocked != 0 || got.Open != 0 || len(got.Rows) != 0 {
				t.Fatalf("a workspace with %s must count nothing, got %+v", tc.name, got)
			}
		})
	}
}

// A pinned finding is a defect pinned only when it lands on the case's planted defect, read by the same matcher a
// review's findings are read by. A row that pins something else, or one that recorded no pin, marks nothing.
func TestPinnedDefectsMarksTheDefectAPinnedRowNames(t *testing.T) {
	key := Key{ID: "case-a", Defects: []Defect{
		{ID: "d1", File: "src/a.mjs", Line: 1, Keywords: []string{"overflow"}},
		{ID: "d2", File: "src/b.mjs", Line: 40, Keywords: []string{"bounds"}},
	}}
	ws := chainWS(t, ""+
		"| P1 | src/a.mjs:1 | add returns a wrong sum | pinned | tests/a.test.mjs :: adds | - |\n"+
		"| P2 | src/z.mjs:12 | nothing is planted here | pinned | tests/z.test.mjs :: unrelated | - |\n"+
		"| P3 | src/b.mjs:40 | the bound is off by one | blocked | - | the module cannot be imported |\n")
	pinned := PinnedDefects(ws, key)
	if len(pinned) != 1 || !pinned["d1"] {
		t.Fatalf("pinned defects = %v, want only d1: an unrelated location pins nothing of this case, and a blocked row pinned nothing at all", pinned)
	}
}

// The prompt is the arm's whole instruction: where the review's findings are, and the three commands that record
// what happened to each one. A pin nobody recorded is a pin the run did not make.
func TestPinPromptNamesTheFindingsAndTheThreeCommands(t *testing.T) {
	path := filepath.Join("bench", "results", "r1", "reviewers.jsonl")
	got := PinPrompt(path)
	for _, want := range []string{
		path,
		"tsp pin --record <id> --test '<suite path> :: <test name>'",
		"tsp pin --block <id> --reason",
		"tsp pin --refute <id> --reason",
		"fails on this code and passes on the fixed code",
		"Do not change any source file.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("the prompt does not say %q:\n%s", want, got)
		}
	}
}

// --pin-from is the chain arm: the agent is handed the review's findings instead of the bench prompt, and what it
// recorded in the Pinning section is scored onto the case, beside detection rather than inside it.
func TestPinFromHandsTheAgentTheFindingsAndScoresThePinning(t *testing.T) {
	requireNodeAndGit(t)
	caseDir := fakeCase(t)
	findings := filepath.Join(t.TempDir(), "reviewers.jsonl")
	if err := os.WriteFile(findings, []byte(`{"findings":[{"location":"src/a.mjs:1","claim":"add returns a wrong sum"}]}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var asked string
	agent := func(ctx context.Context, ws string, key Key, opts Options) (AgentResult, error) {
		asked = agentPrompt(key, opts)
		writeFile(t, ws, PlanPath, "# Test plan\n\n## Pinning\n\n"+plancheck.PinningHeader+
			"| P1 | src/a.mjs:1 | add returns a wrong sum | pinned | tests/a.test.mjs :: adds | - |\n"+
			"| P2 | src/b.mjs:40 | nothing is planted here | blocked | - | the module cannot be imported |\n")
		return AgentResult{Result: "pinned one, blocked one", Turns: 2}, nil
	}
	agg, _ := Run(Options{CasesGlob: caseDir, Runs: 1, Timeout: time.Minute, SuiteTimeout: time.Minute,
		Out: t.TempDir(), BenchDir: t.TempDir(), Agent: agent, PinFrom: findings})
	if !strings.Contains(asked, findings) {
		t.Fatalf("the agent was told %q, want the chain's own prompt naming %s", asked, findings)
	}
	if strings.Contains(asked, Prompt) {
		t.Fatalf("the chain arm kept the bench prompt too: %q", asked)
	}
	if len(agg.Cases) != 1 || agg.Cases[0].PinChain == nil {
		t.Fatalf("cases = %+v, want one case carrying its pin_chain", agg.Cases)
	}
	chain := *agg.Cases[0].PinChain
	if chain.Pinned != 1 || chain.Blocked != 1 || chain.Refuted != 0 || chain.Open != 0 {
		t.Fatalf("chain = %+v, want 1 pinned and 1 blocked", chain)
	}
	if len(chain.Defects) != 1 || chain.Defects[0] != "d1" {
		t.Fatalf("pinned defects = %v, want [d1]", chain.Defects)
	}
	if agg.Pinned != 1 || agg.Blocked != 1 || agg.Refuted != 0 {
		t.Fatalf("aggregate = %d pinned, %d blocked, %d refuted; want 1/1/0", agg.Pinned, agg.Blocked, agg.Refuted)
	}
}
