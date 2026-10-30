package bench

import (
	"errors"
	"fmt"
)

// ErrGateUnusable marks a gate that could not judge: no aggregate to read, or a threshold nobody set.
// A gate that passes because it read nothing is the failure mode this whole package exists to refuse.
var ErrGateUnusable = errors.New("bench gate unusable")

// Gate is the floor a run has to clear. Both numbers are the operator's: this package never decides
// what "good enough" means, it only refuses to call an empty result good.
type Gate struct {
	MinRecall     float64 // 0 disables the floor
	MaxRegression float64 // 0 disables the baseline comparison
	MaxCostUSD    float64 // 0 disables the spend ceiling
	BaselineDir   string  // a previous results directory; empty skips the comparison
	// SkillVersions accepts a baseline measured with a different skill version. Every other provenance field,
	// and the corpus, still has to match, exactly as compare asks.
	SkillVersions bool
}

// GateVerdict is what the gate measured and why it decided the way it did.
type GateVerdict struct {
	Recall   float64  `json:"recall"`
	Baseline float64  `json:"baseline,omitempty"`
	CostUSD  float64  `json:"cost_usd"`
	Defects  int      `json:"defects"`
	Passed   bool     `json:"passed"`
	Reasons  []string `json:"reasons,omitempty"`
}

// Judge reads one results directory and decides whether it clears the gate. Every reason it refuses is
// named, so a red build says which floor was missed instead of only that something was.
func Judge(resultsDir string, gate Gate) (GateVerdict, error) {
	aggregate, err := readAggregate(resultsDir)
	if err != nil {
		return GateVerdict{}, fmt.Errorf("%w: no aggregate in %s: run the bench first, or point the gate at a run's results directory", ErrGateUnusable, resultsDir)
	}
	verdict := GateVerdict{Recall: aggregate.RecallCaught, CostUSD: aggregate.CostUSD, Defects: aggregate.Defects, Passed: true}
	if aggregate.Defects == 0 {
		return verdict, fmt.Errorf("%w: the run recorded no defect, so its recall is not a measurement", ErrGateUnusable)
	}
	if gate.MaxCostUSD > 0 && aggregate.CostUSD > gate.MaxCostUSD {
		verdict.Passed = false
		verdict.Reasons = append(verdict.Reasons, fmt.Sprintf("cost %.4f is above the ceiling %.4f", aggregate.CostUSD, gate.MaxCostUSD))
	}
	if gate.MinRecall > 0 && aggregate.RecallCaught < gate.MinRecall {
		verdict.Passed = false
		verdict.Reasons = append(verdict.Reasons, fmt.Sprintf("recall %.3f is below the floor %.3f", aggregate.RecallCaught, gate.MinRecall))
	}
	if gate.BaselineDir != "" {
		baseline, err := readAggregate(gate.BaselineDir)
		if err != nil {
			return verdict, err
		}
		// A baseline is this instrument measured earlier. Subtracting a reading from another model, runner or
		// corpus produces a delta that says nothing about the change, so the gate refuses the pair instead of
		// reporting a pass or a regression it cannot attribute.
		if err := refuseLikeForLike(baseline, aggregate, gate.SkillVersions); err != nil {
			return verdict, fmt.Errorf("%w: the baseline in %s is not a reading of this instrument: %v", ErrGateUnusable, gate.BaselineDir, err)
		}
		verdict.Baseline = baseline.RecallCaught
		if gate.MaxRegression > 0 && baseline.RecallCaught-aggregate.RecallCaught > gate.MaxRegression {
			verdict.Passed = false
			verdict.Reasons = append(verdict.Reasons, fmt.Sprintf("recall %.3f fell %.3f below the baseline %.3f, more than the %.3f allowed", aggregate.RecallCaught, baseline.RecallCaught-aggregate.RecallCaught, baseline.RecallCaught, gate.MaxRegression))
		}
	}
	return verdict, nil
}
