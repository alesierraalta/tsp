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
	assertRoundsAddUp(t, res)
}

// A first round that fails is a round too: a run made with rounds records it, so its rounds add up to what it paid.
func TestFailedFirstRoundIsRecordedAsARound(t *testing.T) {
	agent := &roundsAgent{fail: map[int]bool{0: true}}
	res := runRounds(t, 2, agent).Cases[0]
	if !res.Failed || len(agent.followups) != 1 {
		t.Fatalf("failed = %v after %d calls, want the first round's failure to fail the run before a second round", res.Failed, len(agent.followups))
	}
	if len(res.Rounds) != 1 || res.Rounds[0].Round != 1 || res.Rounds[0].WitnessError == "" {
		t.Fatalf("rounds = %+v, want one round 1 record naming its failure", res.Rounds)
	}
	assertRoundsAddUp(t, res)
}

func assertRoundsAddUp(t *testing.T, res Result) {
	t.Helper()
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
