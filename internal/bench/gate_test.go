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
