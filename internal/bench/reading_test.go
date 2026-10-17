package bench

import (
	"strings"
	"testing"
)

func TestPlanlessHistoryRendering(t *testing.T) {
	input := "| cases | found | recall | caught | no plan | failed | invalid |\n| 9 | 0 | 0.00 | 17 | 9 | 0 | 0 |\n"
	want := "| cases | found | recall | caught | no plan | failed | invalid |\n| 9 | - | - | 17 | - | 0 | 0 |\n" + planlessNote + "\n"
	if got := RenderHistory(input); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestPlanlessRepeatedHistoryRendering(t *testing.T) {
	input := "| cases | runs | reported | caught | no plan |\n| 3 | 3 | 0 | 17 | 9 |\n"
	want := "| cases | runs | reported | caught | no plan |\n| 3 | 3 | - | 17 | - |\n" + planlessNote + "\n"
	if got := RenderHistory(input); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestCaughtDifferenceWithinNoise(t *testing.T) {
	c := Comparison{Before: Aggregate{Caught: 16, Defects: 18}, After: Aggregate{Caught: 15, Defects: 18}}
	want := "caught difference 16/18 vs 15/18 is within noise (95% Wilson/Newcombe interval: -0.295 to +0.187)."
	if got := c.CaughtDifference(); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if !strings.Contains(c.Markdown(), want+"\n") {
		t.Fatal("comparison missing verdict line")
	}
}

func TestCaughtDifferenceAlternateOutcomes(t *testing.T) {
	for _, tc := range []struct {
		a, b Aggregate
		want string
	}{
		{Aggregate{}, Aggregate{}, "caught difference: unavailable (no defect-run denominator)."},
		{Aggregate{Caught: 0, Defects: 100}, Aggregate{Caught: 100, Defects: 100}, "caught difference 0/100 vs 100/100 is outside noise (95% Wilson/Newcombe interval: +0.948 to +1.000)."},
	} {
		if got := (Comparison{Before: tc.a, After: tc.b}).CaughtDifference(); got != tc.want {
			t.Fatalf("got %q want %q", got, tc.want)
		}
	}
}

func TestPlanlessSummary(t *testing.T) {
	a := Aggregate{Cases: []Result{{Case: "x", Total: 2, Caught: 1}}, Defects: 2, Caught: 1}
	want := "| x | - | - | 1/2 | - | - | - | 0.000 | 0 | 0.0 | NO PLAN | - |"
	if !strings.Contains(Summary(a), want+"\n") {
		t.Fatalf("missing %q in %s", want, Summary(a))
	}
}
