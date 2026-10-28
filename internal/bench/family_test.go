package bench

import (
	"strings"
	"testing"
)

// TestPerFamilyCaughtSplitsValidRuns pins the arithmetic: every valid, non-control case-run lands in
// exactly one family bucket, so the per-family defects add up to the aggregate's and an unclassified
// case is never dropped. Invalid, failed and control runs stay out of the denominator, because their
// defects are not part of recall either.
func TestPerFamilyCaughtSplitsValidRuns(t *testing.T) {
	agg := Aggregate{Cases: []Result{
		{Case: "a", Family: "time", Total: 4, Caught: 3},
		{Case: "b", Family: "time", Total: 2, Caught: 2},
		{Case: "c", Family: "contract", Total: 3, Caught: 1},
		{Case: "d", Total: 2, Caught: 2}, // predates families: unclassified
		{Case: "e", Family: "boundary", Total: 5, Caught: 5, Invalid: true},
		{Case: "f", Family: "boundary", Total: 5, Caught: 1, Failed: true},
		{Case: "g", Family: "security", Control: true},
	}}

	got := familyTotals(agg)
	want := []familyTotal{
		{Name: "contract", Caught: 1, Defects: 3},
		{Name: "time", Caught: 5, Defects: 6},
		{Name: unclassifiedFamily, Caught: 2, Defects: 2},
	}
	if len(got) != len(want) {
		t.Fatalf("familyTotals = %+v, want %+v", got, want)
	}
	defects, caught := 0, 0
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("familyTotals[%d] = %+v, want %+v (all: %+v)", i, got[i], want[i], got)
		}
		defects += got[i].Defects
		caught += got[i].Caught
	}
	valid := 0
	for _, c := range agg.Cases {
		if !c.Invalid && !c.Failed && !c.Control {
			valid += c.Total
		}
	}
	if defects != valid {
		t.Fatalf("per-family defects = %d, want the %d valid defect-runs", defects, valid)
	}
	if caught != 8 {
		t.Fatalf("per-family caught = %d, want 8", caught)
	}
}

// TestSummaryShowsCaughtByFamily keeps both renderers publishing the split: a reading that lost the
// family line would hide which failure kinds the run missed, and the two renderers share the helper.
func TestSummaryShowsCaughtByFamily(t *testing.T) {
	withPlan := Aggregate{Cases: []Result{
		{Case: "a", Family: "time", Total: 4, Caught: 3, PlanFound: true, FindingRows: 1},
		{Case: "b", Family: "time", Total: 2, Caught: 2, PlanFound: true, FindingRows: 1},
	}}
	planless := Aggregate{Cases: []Result{
		{Case: "a", Family: "time", Total: 4, Caught: 3},
		{Case: "b", Total: 2, Caught: 2},
	}}

	for name, text := range map[string]string{"plan": Summary(withPlan), "planless": Summary(planless)} {
		if !strings.Contains(text, "caught by family:") {
			t.Fatalf("%s summary has no family line:\n%s", name, text)
		}
	}
	if got := Summary(withPlan); !strings.Contains(got, "time 5/6") {
		t.Fatalf("plan summary does not report time 5/6:\n%s", got)
	}
	if got := Summary(planless); !strings.Contains(got, unclassifiedFamily+" 2/2") {
		t.Fatalf("planless summary does not report the unclassified bucket:\n%s", got)
	}
}
