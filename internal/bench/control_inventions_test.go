package bench

import (
	"strings"
	"testing"
)

// A clean control has no keyed defect by construction, so every finding row it produced is an
// invention until someone shows otherwise. Without this counter the bench measured how much a run
// found and never how much it made up, and a harness that invents findings scored like a quiet one.
func TestCleanControlCountsItsFindingRowsAsInventions(t *testing.T) {
	cleanKey := Key{ID: "c01-clean", Language: "node", Suite: "node --test", Control: ControlClean}
	findings := "| F1 | `src/render.js:9` does not escape an embedded quote | M | yes | E1 | open | me | - | - |\n" +
		"| F2 | `src/render.js:22` leaks the buffer on the error path | M | yes | E2 | open | me | - | - |\n"

	result := Score(plan(findings, ""), cleanKey)
	if result.ControlInventions != 2 {
		t.Fatalf("control inventions = %d, want both finding rows counted", result.ControlInventions)
	}
	if result.FindingRows != 2 {
		t.Fatalf("finding rows = %d, want 2", result.FindingRows)
	}
	// The claim is still pending a decision: an invention count is not an adjudication, and the
	// existing rule that an unmatched row is not a false positive before adjudication stands.
	if result.FalsePositives != 0 {
		t.Fatalf("false positives = %d, want 0 before adjudication", result.FalsePositives)
	}
	if result.PendingAdjudication != 2 {
		t.Fatalf("pending adjudication = %d, want 2", result.PendingAdjudication)
	}
}

// Adjudicating a control row does not change the invention count: the count says how many claims
// were made about code the corpus calls correct, and a decision only says what one claim was worth.
func TestAdjudicatingAControlRowLeavesTheInventionCountAlone(t *testing.T) {
	cleanKey := Key{ID: "c01-clean", Language: "node", Suite: "node --test", Control: ControlClean}
	findings := "| F1 | `src/render.js:9` does not escape an embedded quote | M | yes | E1 | open | me | - | - |\n" +
		"| F2 | `src/render.js:22` leaks the buffer on the error path | M | yes | E2 | open | me | - | - |\n"

	planText := plan(findings, "")
	fingerprint, err := FindingRowFingerprint(planText, 1)
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	adj := &Adjudication{Case: cleanKey.ID, Run: 1, Decisions: []Decision{{
		Row: 1, Verdict: VerdictFalsePositive, Reason: "the code is correct here", By: "reviewer", RowFingerprint: fingerprint,
	}}}
	result, err := ScoreAdjudicated(planText, cleanKey, adj)
	if err != nil {
		t.Fatalf("adjudicate: %v", err)
	}
	if result.ControlInventions != 2 {
		t.Fatalf("control inventions = %d, want both rows still named as claims", result.ControlInventions)
	}
	if result.FalsePositives != 1 || result.PendingAdjudication != 1 {
		t.Fatalf("false positives = %d, pending = %d, want the decided row counted and the other left open", result.FalsePositives, result.PendingAdjudication)
	}
}

// The aggregate carries the count and the totals line names it, so an arm that invents findings can
// never look like an arm that stayed quiet.
func TestAggregateTotalsNameControlInventions(t *testing.T) {
	agg := Aggregate{Cases: []Result{
		{Case: "c01-clean", Run: 1, Control: true, PlanFound: true, FindingRows: 2, ControlInventions: 2},
		{Case: "c02-clean", Run: 1, Control: true, PlanFound: true},
	}}
	finalizeAggregate(&agg)
	if agg.Controls != 2 || agg.ControlInventions != 2 {
		t.Fatalf("controls = %d, inventions = %d, want 2 and 2", agg.Controls, agg.ControlInventions)
	}
	if line := aggregateTotals(agg); !strings.Contains(line, "inventions on controls: 2 finding rows") {
		t.Fatalf("totals = %s, want the invention count named", line)
	}
}

// A quiet clean control is the pass condition and reads as zero inventions, not as an undefined ratio.
func TestQuietCleanControlScoresZeroInventions(t *testing.T) {
	cleanKey := Key{ID: "g05-clean", Language: "go", Suite: "go test ./...", Control: ControlClean}
	result := Score(plan("", ""), cleanKey)
	if result.ControlInventions != 0 || result.FindingRows != 0 {
		t.Fatalf("control inventions = %d, rows = %d, want a quiet control", result.ControlInventions, result.FindingRows)
	}
	if result.Precision != nil {
		t.Fatalf("precision = %v, want undefined when no claim was made", *result.Precision)
	}
}

// The case table must not print "clean" for a control that invented findings.
func TestSummarySaysHowManyFindingsAControlInvented(t *testing.T) {
	summary := Summary(Aggregate{Cases: []Result{
		{Case: "c01-clean", Run: 1, Control: true, PlanFound: true, FindingRows: 2, ControlInventions: 2},
		{Case: "c02-clean", Run: 1, Control: true, PlanFound: true},
	}})
	if !strings.Contains(summary, "| c01-clean | clean, 2 invented |") {
		t.Fatalf("summary = %s, want the invented count in the control row", summary)
	}
	if !strings.Contains(summary, "| c02-clean | clean |") {
		t.Fatalf("summary = %s, want a quiet control to keep the plain word", summary)
	}
}
