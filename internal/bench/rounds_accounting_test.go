package bench

import "testing"

// A later round that fails still cost turns and money: its record keeps that work, so the rounds of a failed run add
// up to what the run paid.
func TestFailedLaterRoundRetainsPerRoundWorkAccounting(t *testing.T) {
	agent := &roundsAgent{fail: map[int]bool{1: true}}
	res := runRounds(t, 2, agent).Cases[0]
	if !res.Failed {
		t.Fatal("expected the later round's infrastructure failure to fail the run")
	}
	var cost float64
	var turns int
	for _, round := range res.Rounds {
		cost += round.CostUSD
		turns += round.Turns
	}
	if cost != res.CostUSD || turns != res.Turns {
		t.Fatalf("per-round work omits a failed round: recorded cost %g and turns %d; run paid cost %g and turns %d", cost, turns, res.CostUSD, res.Turns)
	}
}
