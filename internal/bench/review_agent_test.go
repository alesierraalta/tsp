package bench

import (
	"strings"
	"testing"
)

// A reviewer's answer is scored by the same matcher a test-writing run's plan goes through, so the two arms of a
// measurement are read alike (#410).
func TestScoreReviewTextReadsTheReviewersFindings(t *testing.T) {
	key := Key{ID: "f01", Defects: []Defect{
		{ID: "D1", File: "src/renewal.js", Line: 3, Keywords: []string{"month", "clamp"}},
		{ID: "D2", File: "src/renewal.js", Line: 9, Keywords: []string{"leap"}},
	}}
	answer := "Here is what I found.\n{\"findings\":[{\"location\":\"src/renewal.js:3\",\"claim\":\"the month end is not clamped\",\"severity\":\"high\"}]}"
	r, err := ScoreReviewText(answer, key)
	if err != nil {
		t.Fatal(err)
	}
	if r.Detected != 1 || r.Total != 2 || r.FindingRows != 1 {
		t.Fatalf("scored %+v; want D1 detected and one row", r)
	}
	if r.Defects[0].MatchedBy == "" || r.Defects[1].MatchedBy != "" {
		t.Fatalf("defects = %+v", r.Defects)
	}
	quiet, err := ScoreReviewText(`{"findings":[]}`, key)
	if err != nil || quiet.Detected != 0 || quiet.FindingRows != 0 {
		t.Fatalf("an empty answer = %+v, %v; want nothing found and nothing claimed", quiet, err)
	}
	if _, err := ScoreReviewText("I could not review this", key); err == nil {
		t.Fatal("an answer with no findings object must be an error, not a review with nothing found")
	}
}

// Cuantos turns and cost Pi reports are kept, so a reviewer arm is comparable on cost as well as on detection.
func TestParsePiAnswerKeepsTheTextCostAndTurns(t *testing.T) {
	out := strings.Join([]string{
		`{"type":"message","content":[{"type":"text","text":"first"}]}`,
		`{"type":"result","result":"{\"findings\":[]}","total_cost_usd":0.004,"num_turns":7,"duration_ms":1234}`,
	}, "\n")
	ar := parsePiAnswer([]byte(out))
	if ar.Result != `{"findings":[]}` || ar.CostUSD != 0.004 || ar.Turns != 7 || ar.DurationMS != 1234 {
		t.Fatalf("parsePiAnswer = %+v", ar)
	}
}

func TestReviewIsAKnownRunner(t *testing.T) {
	if !KnownRunner(RunnerReview) {
		t.Fatal("review must be a known runner")
	}
}
