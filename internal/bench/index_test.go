package bench

import (
	"strings"
	"testing"
)

// TestIndexWeighsRecallBreadthAndPrecision pins the arithmetic on a reading with no control runs: the
// weights renormalize over the parts that exist, so a reading is never scored as if it had measured
// precision. Two defects caught of four, across one of two families: 0.5*0.5 + 0.3*0.5 over 0.8 = 50.
func TestIndexWeighsRecallBreadthAndPrecision(t *testing.T) {
	agg := Aggregate{Cases: []Result{
		{Case: "a", Family: "time", Total: 2, Caught: 2},
		{Case: "b", Family: "security", Total: 2},
	}}
	got := indexOf(agg)
	if !got.Scored {
		t.Fatal("indexOf scored nothing on a reading with defects")
	}
	if got.Recall != 0.5 || got.Breadth != 0.5 {
		t.Fatalf("recall %v breadth %v, want 0.5 and 0.5", got.Recall, got.Breadth)
	}
	if got.Score != 50 {
		t.Fatalf("score = %v, want 50", got.Score)
	}
	if text := indexSection(agg); !strings.Contains(text, "50/100") || !strings.Contains(text, "precision not measured") {
		t.Fatalf("index line does not publish the score and the missing part: %q", text)
	}
}

// TestInventingControlsCapsTheIndex keeps recall from buying precision: a reading that caught everything
// still loses a fifth of the score for inventing a defect on clean code, because an invented defect costs
// a reviewer the same trust as a missed one.
func TestInventingControlsCapsTheIndex(t *testing.T) {
	caughtEverywhere := Aggregate{Cases: []Result{
		{Case: "a", Family: "time", Total: 2, Caught: 2},
		{Case: "b", Family: "security", Total: 2, Caught: 2},
		{Case: "c", Family: "security", Control: true},
		{Case: "d", Family: "security", Control: true, ControlInventions: 1},
	}}
	got := indexOf(caughtEverywhere)
	if got.Recall != 1 || got.Breadth != 1 {
		t.Fatalf("recall %v breadth %v, want 1 and 1", got.Recall, got.Breadth)
	}
	if got.Precision != 0.5 || got.Controls != 2 {
		t.Fatalf("precision %v over %d control runs, want 0.5 over 2", got.Precision, got.Controls)
	}
	if got.Score != 90 {
		t.Fatalf("score = %v, want 90: 0.5*1 + 0.3*1 + 0.2*0.5 over 1.0", got.Score)
	}
	if got.Score >= 100 {
		t.Fatal("an invented control defect must not score a perfect reading")
	}
}

// TestIndexPublishesInBothRenderers: a plan-less reading still carries the index, so the number a reader
// compares against a later reading exists whether or not the run recorded a plan.
func TestIndexPublishesInBothRenderers(t *testing.T) {
	planless := Aggregate{Cases: []Result{{Case: "a", Family: "time", Total: 2, Caught: 2}}}
	withPlan := Aggregate{Cases: []Result{{Case: "a", Family: "time", Total: 2, Caught: 2, PlanFound: true, FindingRows: 1}}}
	for name, text := range map[string]string{"plan-less": Summary(planless), "plan": Summary(withPlan)} {
		if !strings.Contains(text, "index: 100/100") {
			t.Fatalf("%s summary does not publish the index: %q", name, text)
		}
	}
}

// TestIndexRefusesToScoreAnEmptyReading: a reading with no defect-runs measured nothing, and a 0 there
// would read as a total failure instead of a missing measurement.
func TestIndexRefusesToScoreAnEmptyReading(t *testing.T) {
	empty := Aggregate{Cases: []Result{{Case: "c", Family: "security", Control: true}}}
	if got := indexOf(empty); got.Scored || indexSection(empty) != "" {
		t.Fatalf("an empty reading scored %+v", got)
	}
}
