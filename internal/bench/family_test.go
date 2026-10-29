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

// TestPerFamilyCaughtPublishesAFamilyOutsideTheList pins the branch a key from an older or hand-edited
// case takes: a name the list does not know is still published, in a stable order and before the
// unclassified bucket, so a caught defect never disappears from the split.
func TestPerFamilyCaughtPublishesAFamilyOutsideTheList(t *testing.T) {
	agg := Aggregate{Cases: []Result{
		{Case: "a", Family: "time", Total: 1, Caught: 1},
		{Case: "b", Family: "zzz-older", Total: 1, Caught: 0},
		{Case: "c", Family: "typo-family", Total: 2, Caught: 1},
		{Case: "d", Total: 1, Caught: 1},
	}}
	got := familyTotals(agg)
	want := []familyTotal{
		{Name: "time", Caught: 1, Defects: 1},
		{Name: "typo-family", Caught: 1, Defects: 2},
		{Name: "zzz-older", Caught: 0, Defects: 1},
		{Name: unclassifiedFamily, Caught: 1, Defects: 1},
	}
	if len(got) != len(want) {
		t.Fatalf("familyTotals = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("familyTotals[%d] = %+v, want %+v (all: %+v)", i, got[i], want[i], got)
		}
	}
	section := familySection(agg)
	for _, name := range []string{"typo-family", "zzz-older"} {
		if !strings.Contains(section, name) {
			t.Fatalf("the split lost %q: %s", name, section)
		}
	}
}
