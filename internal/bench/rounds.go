package bench

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// RoundResult is one round of a run made with Options.Rounds: what the workspace held once that round ended.
// Turns, cost and seconds are the round's own agent work; the checks between rounds are not counted in them.
type RoundResult struct {
	Round        int      `json:"round"`
	Witnesses    []string `json:"witnesses"`               // tests failing on the unmodified code once the round ended
	WitnessError string   `json:"witness_error,omitempty"` // why the witnesses could not be read; they count as none
	NewWitnesses int      `json:"new_witnesses"`           // of those, tests no earlier round left failing
	Caught       int      `json:"caught"`                  // defects the workspace's tests distinguish once the round ended
	BrokenTests  int      `json:"broken_tests"`            // tests red on the fully fixed code once the round ended
	Turns        int      `json:"turns"`
	CostUSD      float64  `json:"cost_usd"`
	Seconds      float64  `json:"seconds"`
}

// roundFollowup is what a later round is told: the tests earlier rounds left failing on the unmodified code, each
// pinning a defect, so it looks for defects those tests do not cover. Finding nothing new is said to be a valid
// answer, so asking for another round does not push the agent to invent a defect.
func roundFollowup(witnesses []string) string {
	if len(witnesses) == 0 {
		return "An earlier round of this task left no test failing on the unmodified code, so no defect is pinned yet. " +
			"Look again for defects the suite misses; finding none is a valid result."
	}
	var b strings.Builder
	b.WriteString("Earlier rounds of this task left these tests failing on the unmodified code; each pins a defect:\n")
	for _, name := range witnesses {
		fmt.Fprintf(&b, "- %s\n", name)
	}
	b.WriteString("Keep those tests as they are and do not pin the same defects again: look for defects they do not cover. " +
		"Finding no new defect is a valid result.")
	return b.String()
}

// recordRound appends the round that just ended to res and returns its witnesses: the tests the workspace leaves
// failing on the unmodified code, how many of them no earlier round left, what its tests catch, and the turns and
// cost the round alone added. A workspace whose tests cannot be judged one by one records why and no witness.
func recordRound(res *Result, ws string, catch CatchResult, seconds float64, timeout time.Duration) []string {
	r := RoundResult{Round: len(res.Rounds) + 1, Witnesses: []string{}, Caught: catch.Count(), BrokenTests: catch.BrokenTests,
		Turns: res.Turns, CostUSD: res.CostUSD, Seconds: seconds}
	seen := map[string]bool{}
	for _, prev := range res.Rounds {
		r.Turns -= prev.Turns
		r.CostUSD -= prev.CostUSD
		for _, w := range prev.Witnesses {
			seen[w] = true
		}
	}
	if rep, err := RedCheck(ws, "", timeout); err != nil {
		r.WitnessError = err.Error()
	} else {
		r.Witnesses = append(r.Witnesses, rep.Red...)
	}
	for _, w := range r.Witnesses {
		if !seen[w] {
			r.NewWitnesses++
		}
	}
	res.Rounds = append(res.Rounds, r)
	return r.Witnesses
}

// addRounds adds one valid run's per-round catches and broken tests to the reading's per-round totals.
func addRounds(agg *Aggregate, rounds []RoundResult) {
	for i, r := range rounds {
		if i == len(agg.CaughtByRound) {
			agg.CaughtByRound = append(agg.CaughtByRound, 0)
			agg.BrokenByRound = append(agg.BrokenByRound, 0)
		}
		agg.CaughtByRound[i] += r.Caught
		agg.BrokenByRound[i] += r.BrokenTests
	}
}

// roundsTotals is the summary's per-round line for a reading whose runs made rounds; empty for a one-pass reading.
func roundsTotals(a Aggregate) string {
	if len(a.CaughtByRound) == 0 {
		return ""
	}
	return fmt.Sprintf(" · caught after each round (defect-runs): %s of %d · tests red on the fixed code after each round: %s",
		joinInts(a.CaughtByRound), a.Defects, joinInts(a.BrokenByRound))
}

func joinInts(values []int) string {
	parts := make([]string, len(values))
	for i, v := range values {
		parts[i] = strconv.Itoa(v)
	}
	return strings.Join(parts, ", ")
}

// passes reads a recorded round count: a reading that recorded none made one pass per run.
func passes(rounds int) int {
	if rounds < 1 {
		return 1
	}
	return rounds
}
