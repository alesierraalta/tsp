package bench

import (
	"encoding/json"
	"fmt"
	"strings"
)

// reviewFindingsHeader is the plan template's Findings header, so review findings are read by the very matcher a
// test arm's plan is read by.
const reviewFindingsHeader = "| Id | Finding (path:line, one line) | Severity (consequence class) | Data safe? | Evidence id | Pinning test (suite path :: test name) | Status | Verdict by / date | Reason | Cited-files fingerprint at verdict |\n|---|---|---|---|---|---|---|---|---|---|\n"

// ScoreReview scores a gentle-review run from the reviewer calls it logged. Each finding becomes one Findings row
// (its location and claim) and the rows go through Score, so a review detection and a plan detection mean the same
// thing: a claim that cites the defect's file at its line, or names it. Lenses that report the same location are one
// claim, because Gentle AI folds its lenses into one review. A review writes no test, so nothing here is pinned.
func ScoreReview(reviewers []byte, key Key) Result {
	var rows strings.Builder
	seen := map[string]bool{}
	n := 0
	for _, line := range strings.Split(strings.TrimSpace(string(reviewers)), "\n") {
		var call reviewerCall
		if json.Unmarshal([]byte(line), &call) != nil {
			continue
		}
		var answer struct {
			Findings []struct {
				Location string `json:"location"`
				Severity string `json:"severity"`
				Claim    string `json:"claim"`
			} `json:"findings"`
		}
		if json.Unmarshal([]byte(call.Result), &answer) != nil {
			continue
		}
		for _, f := range answer.Findings {
			where := strings.TrimSpace(f.Location)
			if seen[where] {
				continue
			}
			seen[where] = true
			n++
			fmt.Fprintf(&rows, "| R%d | %s %s | %s | - | - | - | confirmed | review | - | - |\n", n, cellText(where), cellText(f.Claim), cellText(f.Severity))
		}
	}
	r := Score("## Findings\n\n"+reviewFindingsHeader+rows.String(), key)
	// The plan matcher credits a row only when it links executed ledger evidence, because a plan's claim must have
	// been observed. A review executes nothing, so its claims never link: a matched claim is the review's detection,
	// and DetectedDefects is the measure a review and a plan share. Found stays the evidence-backed measure.
	r.Detected = DetectedDefects(r)
	r.Notes = append(r.Notes, fmt.Sprintf("review: %d distinct finding(s) scored with the plan matcher; %d of %d defects detected, none evidence-backed", n, r.Detected, r.Total))
	return r
}

// DetectedDefects counts the defects a scored plan or review names, with or without linked evidence: the measure two
// arms share when only one of them executes what it claims.
func DetectedDefects(r Result) int {
	n := 0
	for _, d := range r.Defects {
		if d.MatchedBy != "" {
			n++
		}
	}
	return n
}

// cellText keeps one finding on one table row: a pipe would open a column and a newline would end the row.
func cellText(s string) string {
	s = strings.ReplaceAll(s, "|", "/")
	return strings.Join(strings.Fields(s), " ")
}
