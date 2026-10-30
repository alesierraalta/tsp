package bench

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func gateRun(t *testing.T, dir string, agg Aggregate) string {
	t.Helper()
	writeJSON(filepath.Join(dir, "aggregate.json"), agg)
	return dir
}

func TestGatePassesARunAboveEveryFloor(t *testing.T) {
	dir := gateRun(t, t.TempDir(), Aggregate{Defects: 10, RecallCaught: 0.8, CostUSD: 0.4})

	verdict, err := Judge(dir, Gate{MinRecall: 0.7, MaxCostUSD: 1})
	if err != nil {
		t.Fatalf("Judge: %v", err)
	}
	if !verdict.Passed || len(verdict.Reasons) != 0 {
		t.Fatalf("verdict = %+v, want a pass with no reason", verdict)
	}
}

func TestGateNamesEveryFloorItMissed(t *testing.T) {
	dir := gateRun(t, t.TempDir(), Aggregate{Defects: 10, RecallCaught: 0.5, CostUSD: 2.5})

	verdict, err := Judge(dir, Gate{MinRecall: 0.7, MaxCostUSD: 1})
	if err != nil {
		t.Fatalf("Judge: %v", err)
	}
	if verdict.Passed {
		t.Fatal("a run below the floor and above the ceiling passed")
	}
	joined := strings.Join(verdict.Reasons, "; ")
	for _, want := range []string{"recall 0.500 is below the floor 0.700", "cost 2.5000 is above the ceiling 1.0000"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("reasons = %q, want %q", joined, want)
		}
	}
}

func TestGateRefusesARunThatMeasuredNoDefect(t *testing.T) {
	dir := gateRun(t, t.TempDir(), Aggregate{Defects: 0, RecallCaught: 0, CostUSD: 0})

	_, err := Judge(dir, Gate{MinRecall: 0.7})
	if !errors.Is(err, ErrGateUnusable) {
		t.Fatalf("err = %v, want ErrGateUnusable: a run with no defect is not a zero score, it is no measurement", err)
	}
}

func TestGateRefusesAMissingAggregate(t *testing.T) {
	_, err := Judge(t.TempDir(), Gate{MinRecall: 0.7})
	if !errors.Is(err, ErrGateUnusable) {
		t.Fatalf("err = %v, want ErrGateUnusable", err)
	}
}

// A baseline is the same instrument measured earlier. Anything else is a second experiment whose number is
// not this reading's past, so the gate refuses it the way compare refuses it rather than subtracting them.
func TestGateRefusesABaselineFromAnotherModel(t *testing.T) {
	baseline := gateRun(t, t.TempDir(), Aggregate{Defects: 10, RecallCaught: 0.727, Model: "openai-codex/gpt-6-luna"})
	current := gateRun(t, t.TempDir(), Aggregate{Defects: 10, RecallCaught: 0.913, Model: "gemini-3.8-flash-low"})

	_, err := Judge(current, Gate{MaxRegression: 0.05, BaselineDir: baseline})
	if !errors.Is(err, ErrGateUnusable) {
		t.Fatalf("err = %v, want ErrGateUnusable: another model's reading is not this reading's baseline", err)
	}
	if !strings.Contains(err.Error(), "different model: before openai-codex/gpt-6-luna, after gemini-3.8-flash-low") {
		t.Fatalf("err = %v, want the model named", err)
	}
}

func TestGateRefusesABaselineFromAnotherRunner(t *testing.T) {
	baseline := gateRun(t, t.TempDir(), Aggregate{Defects: 10, RecallCaught: 0.8, Provenance: Provenance{Runner: "pi"}})
	current := gateRun(t, t.TempDir(), Aggregate{Defects: 10, RecallCaught: 0.8, Provenance: Provenance{Runner: "agy"}})

	_, err := Judge(current, Gate{MaxRegression: 0.05, BaselineDir: baseline})
	if !errors.Is(err, ErrGateUnusable) {
		t.Fatalf("err = %v, want ErrGateUnusable", err)
	}
	if !strings.Contains(err.Error(), "different runner: before pi, after agy") {
		t.Fatalf("err = %v, want the runner named", err)
	}
}

func TestGateRefusesABaselineFromAnotherCorpus(t *testing.T) {
	baseline := gateRun(t, t.TempDir(), Aggregate{Defects: 10, RecallCaught: 0.8, Corpus: "sha256:aaa"})
	current := gateRun(t, t.TempDir(), Aggregate{Defects: 10, RecallCaught: 0.8, Corpus: "sha256:bbb"})

	_, err := Judge(current, Gate{MaxRegression: 0.05, BaselineDir: baseline})
	if !errors.Is(err, ErrGateUnusable) {
		t.Fatalf("err = %v, want ErrGateUnusable", err)
	}
	if !strings.Contains(err.Error(), "different corpora: before sha256:aaa, after sha256:bbb") {
		t.Fatalf("err = %v, want both corpora named", err)
	}
}

// The one provenance field the operator may wave through deliberately, in the shape compare already offers.
func TestGateWavesASkillVersionThroughOnlyWhenAsked(t *testing.T) {
	baseline := gateRun(t, t.TempDir(), Aggregate{Defects: 10, RecallCaught: 0.8, SkillVersion: "0.7.11"})
	current := gateRun(t, t.TempDir(), Aggregate{Defects: 10, RecallCaught: 0.85, SkillVersion: "0.7.12"})

	if _, err := Judge(current, Gate{MaxRegression: 0.05, BaselineDir: baseline}); err == nil {
		t.Fatal("a different skill version was compared without being asked for")
	}
	verdict, err := Judge(current, Gate{MaxRegression: 0.05, BaselineDir: baseline, SkillVersions: true})
	if err != nil {
		t.Fatalf("Judge with SkillVersions: %v", err)
	}
	if !verdict.Passed || verdict.Baseline != 0.8 {
		t.Fatalf("verdict = %+v, want a pass against the 0.8 baseline", verdict)
	}
}

func TestGateRefusesARegressionAgainstTheBaseline(t *testing.T) {
	baseline := gateRun(t, t.TempDir(), Aggregate{Defects: 10, RecallCaught: 0.9})
	current := gateRun(t, t.TempDir(), Aggregate{Defects: 10, RecallCaught: 0.6})

	verdict, err := Judge(current, Gate{MaxRegression: 0.1, BaselineDir: baseline})
	if err != nil {
		t.Fatalf("Judge: %v", err)
	}
	if verdict.Passed || verdict.Baseline != 0.9 {
		t.Fatalf("verdict = %+v, want a failure that names the baseline", verdict)
	}
	if !strings.Contains(strings.Join(verdict.Reasons, "; "), "fell 0.300 below the baseline 0.900") {
		t.Fatalf("reasons = %v, want the regression named", verdict.Reasons)
	}
}
