package bench

import (
	"encoding/json"
	"strings"
	"testing"
)

func reviewerLog(t *testing.T, lenses ...[]map[string]string) []byte {
	t.Helper()
	var b strings.Builder
	for i, findings := range lenses {
		result, _ := json.Marshal(map[string]any{"lens": "review-" + []string{"risk", "reliability", "resilience", "readability"}[i%4], "findings": findings})
		line, _ := json.Marshal(reviewerCall{Model: "haiku", Result: string(result)})
		b.Write(line)
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

func renewalKey() Key {
	return Key{ID: "f01", Defects: []Defect{
		{ID: "D1", File: "src/renewal.js", Line: 3, Keywords: []string{"month", "clamp"}},
		{ID: "D2", File: "src/renewal.js", Line: 9, Keywords: []string{"leap"}},
	}}
}

// A review finding is judged by the same matcher the plan's Findings are: a finding that cites the defect's file and
// line is a detection, the same finding repeated by another lens is one claim, and a finding no defect explains is
// counted unmatched.
func TestScoreReviewMatchesFindingsWithThePlanMatcher(t *testing.T) {
	log := reviewerLog(t,
		[]map[string]string{{"location": "src/renewal.js:3", "claim": "monthlyRenewal no longer clamps the day of month"}},
		[]map[string]string{
			{"location": "src/renewal.js:3", "claim": "monthly renewal overflows the destination month"},
			{"location": "src/other.js:40", "claim": "an unrelated style concern"},
		},
	)
	r := ScoreReview(log, renewalKey())
	if r.Detected != 1 || r.Total != 2 || r.Found != 0 {
		t.Fatalf("detected %d found %d of %d; want D1 detected and nothing evidence-backed", r.Detected, r.Found, r.Total)
	}
	if r.Defects[0].MatchedBy == "" || r.Defects[1].MatchedBy != "" {
		t.Fatalf("defects = %+v; want D1 matched, D2 missed", r.Defects)
	}
	if r.FindingRows != 2 || r.UnmatchedFindings != 1 {
		t.Fatalf("rows %d unmatched %d; want the repeated D1 finding read once and one unmatched finding", r.FindingRows, r.UnmatchedFindings)
	}
}

// On a clean control nothing is keyed, so every finding the review raises is an invention; a quiet review raises none.
func TestScoreReviewCountsFindingsOnACleanControlAsInventions(t *testing.T) {
	control := Key{ID: "c01", Control: ControlClean}
	r := ScoreReview(reviewerLog(t, []map[string]string{{"location": "src/a.js:1", "claim": "possible race"}}), control)
	if r.UnmatchedFindings != 1 {
		t.Fatalf("unmatched = %d, want the control finding counted", r.UnmatchedFindings)
	}
	if quiet := ScoreReview(reviewerLog(t, []map[string]string{}), control); quiet.UnmatchedFindings != 0 || quiet.FindingRows != 0 {
		t.Fatalf("a quiet review scored %+v", quiet)
	}
}

// A reviewer answer that is not JSON names nothing the matcher can read, so it detects nothing rather than failing the run.
func TestScoreReviewReadsAnUnparseableAnswerAsNoFinding(t *testing.T) {
	line, _ := json.Marshal(reviewerCall{Model: "haiku", Result: "I could not review this."})
	if r := ScoreReview(append(line, '\n'), renewalKey()); r.Detected != 0 || r.FindingRows != 0 {
		t.Fatalf("unparseable answer scored %+v", r)
	}
}
