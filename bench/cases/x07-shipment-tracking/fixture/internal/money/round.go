package money

import "fmt"

// divRoundHalfEven divides num by den, rounding the quotient to the nearest integer and the exact
// half to the even one. den is positive and num may be negative; the quotient is truncated toward
// zero first, so -5/2 is -2 truncated and rounds to -2, and -7/2 is -3 truncated with a remainder of
// one half, which rounds away from zero to -4. The half test compares the remainder with den-r
// instead of doubling it, so a large denominator cannot overflow the comparison.
func divRoundHalfEven(num, den int64) (int64, error) {
	if den <= 0 {
		return 0, fmt.Errorf("%w: denominator %d", ErrRounding, den)
	}
	quotient := num / den
	remainder := num % den
	if remainder == 0 {
		return quotient, nil
	}
	step := int64(1)
	if remainder < 0 {
		remainder = -remainder
		step = -1
	}
	rest := den - remainder
	switch {
	case remainder > rest:
		return quotient + step, nil
	case remainder < rest:
		return quotient, nil
	}
	if quotient%2 != 0 {
		return quotient + step, nil
	}
	return quotient, nil
}

// Split divides total across len(weights) shares in proportion to the weights, and makes the shares
// add back to total exactly. The shares are whole minor units, so the parts that proportional
// division cannot place — at most one minor unit each — go to the shares with the largest remainders,
// highest weight first on a tie. A negative total is split by magnitude and the sign is applied to
// every share, and a zero total returns zero shares. A zero or negative weight, or weights that sum
// to zero, is refused: a share of nothing cannot be proportional to nothing.
func Split(total Amount, weights []int64) ([]Amount, error) {
	if len(weights) == 0 {
		return nil, fmt.Errorf("%w: no shares", ErrAllocate)
	}
	var sumWeights int64
	for _, weight := range weights {
		if weight < 0 {
			return nil, fmt.Errorf("%w: negative weight %d", ErrAllocate, weight)
		}
		next, ok := addChecked(sumWeights, weight)
		if !ok {
			return nil, fmt.Errorf("%w: weights out of range", ErrAllocate)
		}
		sumWeights = next
	}
	if sumWeights == 0 {
		return nil, fmt.Errorf("%w: weights sum to zero", ErrAllocate)
	}
	magnitude, err := total.Abs()
	if err != nil {
		return nil, err
	}
	shares := make([]int64, len(weights))
	remainders := make([]int64, len(weights))
	var placed int64
	for i, weight := range weights {
		scaled, ok := mulChecked(int64(magnitude), weight)
		if !ok {
			return nil, fmt.Errorf("%w: share %d out of range", ErrOverflow, i)
		}
		shares[i] = scaled / sumWeights
		remainders[i] = scaled % sumWeights
		placed += shares[i]
	}
	// The leftover is smaller than the number of shares: each share held back less than one minor unit.
	leftover := int64(magnitude) - placed
	for leftover > 0 {
		best := -1
		for i := range shares {
			if remainders[i] <= 0 {
				continue
			}
			if best < 0 || remainders[i] > remainders[best] || (remainders[i] == remainders[best] && weights[i] > weights[best]) {
				best = i
			}
		}
		if best < 0 {
			// No remainder is left to place, which cannot happen while leftover is smaller than the number
			// of shares and every share was computed from the same denominator; refuse instead of guessing.
			return nil, fmt.Errorf("%w: %d minor units unplaced", ErrAllocate, leftover)
		}
		shares[best]++
		remainders[best] = 0
		leftover--
	}
	result := make([]Amount, len(shares))
	for i, share := range shares {
		if total.Negative() {
			result[i] = Amount(-share)
		} else {
			result[i] = Amount(share)
		}
	}
	return result, nil
}

// SplitCapped divides total across len(weights) shares in proportion to the weights under a ceiling
// per share, and never charges a share more than its ceiling. Unlike Split, the sum is still exactly
// total, so a share that would have gone above its ceiling passes the excess to the shares that still
// have room, largest weight first. A total the ceilings cannot hold is refused rather than truncated:
// an allocation that silently drops a minor unit would make the statement disagree with its total.
func SplitCapped(total Amount, weights []int64, ceilings []Amount) ([]Amount, error) {
	if len(weights) != len(ceilings) {
		return nil, fmt.Errorf("%w: %d weights and %d ceilings", ErrAllocate, len(weights), len(ceilings))
	}
	if len(weights) == 0 {
		return nil, fmt.Errorf("%w: no shares", ErrAllocate)
	}
	magnitude, err := total.Abs()
	if err != nil {
		return nil, err
	}
	for i, ceiling := range ceilings {
		if ceiling < 0 {
			return nil, fmt.Errorf("%w: ceiling %d is negative", ErrAllocate, i)
		}
	}
	var capacity int64
	for _, ceiling := range ceilings {
		next, ok := addChecked(capacity, int64(ceiling))
		if !ok {
			return nil, fmt.Errorf("%w: ceilings out of range", ErrAllocate)
		}
		capacity = next
	}
	if int64(magnitude) > capacity {
		return nil, fmt.Errorf("%w: %s does not fit in %d", ErrAllocate, total, capacity)
	}

	shares := make([]int64, len(weights))
	active := make([]int64, len(weights))
	copy(active, weights)
	remaining := int64(magnitude)
	// Round by round, each round divides what is still unplaced proportionally to the weight of the
	// shares that still have room. A share that reaches its ceiling leaves the next round; the loop
	// ends because every full round either places a unit or empties at least one share.
	for remaining > 0 {
		var weightSum int64
		for i := range shares {
			if int64(shares[i]) >= int64(ceilings[i]) {
				active[i] = 0
			}
			weightSum += active[i]
		}
		if weightSum == 0 {
			return nil, fmt.Errorf("%w: %d minor units unplaced", ErrAllocate, remaining)
		}
		progress := false
		for i := range shares {
			if active[i] == 0 {
				continue
			}
			scaled, ok := mulChecked(remaining, active[i])
			if !ok {
				return nil, fmt.Errorf("%w: share %d out of range", ErrOverflow, i)
			}
			want := scaled / weightSum
			room := int64(ceilings[i]) - shares[i]
			if want > room {
				want = room
			}
			if want <= 0 {
				continue
			}
			shares[i] += want
			remaining -= want
			progress = true
		}
		if !progress {
			// Every share with room rounded to zero: hand one minor unit to the largest weight with room.
			best := -1
			for i := range shares {
				if int64(shares[i]) >= int64(ceilings[i]) {
					continue
				}
				if best < 0 || weights[i] > weights[best] {
					best = i
				}
			}
			if best < 0 {
				return nil, fmt.Errorf("%w: %d minor units unplaced", ErrAllocate, remaining)
			}
			shares[best]++
			remaining--
		}
	}
	result := make([]Amount, len(shares))
	for i, share := range shares {
		if total.Negative() {
			result[i] = Amount(-share)
		} else {
			result[i] = Amount(share)
		}
	}
	return result, nil
}
