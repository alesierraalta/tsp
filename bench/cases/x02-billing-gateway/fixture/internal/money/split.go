package money

import "fmt"

// Split divides total into parts shares of the same currency whose sum is exactly total. The
// remainder of the division is handed out one minor unit at a time to the earliest shares, so a
// cent that exists in the total exists in exactly one share and never appears or disappears.
//
// Split is the only way a total becomes a set of parts in this service: an invoice line, a tax line
// and a credit note are all shares of a total, and a caller that divided on its own would leave the
// cent behind in an account nobody can reconcile.
func Split(total int64, parts int) ([]int64, error) {
	if parts <= 0 {
		return nil, fmt.Errorf("money: split needs a positive number of parts, got %d", parts)
	}
	count := int64(parts)
	base := total / count
	remainder := total % count
	shares := make([]int64, parts)
	for i := range shares {
		shares[i] = base
	}
	step := int64(1)
	if remainder < 0 {
		step = -1
		remainder = -remainder
	}
	for i := int64(0); i < remainder; i++ {
		shares[i] += step
	}
	return shares, nil
}

// SplitEvenly returns the shares of total over the parts given as a slice of amounts of total's
// currency. It is Split with the currency carried along, for callers that add the shares up.
func SplitEvenly(total Amount, parts int) ([]Amount, error) {
	if err := total.Validate(); err != nil {
		return nil, err
	}
	minor, err := Split(total.Minor, parts)
	if err != nil {
		return nil, err
	}
	shares := make([]Amount, len(minor))
	for i, value := range minor {
		shares[i] = Amount{Minor: value, Currency: total.Currency}
	}
	return shares, nil
}

// Prorate returns the part of amount that numer of denom days covers, rounded half up.
//
// The arithmetic stays in integers: a factor is a pair of day counts, never a fraction, so a
// proration of a large amount is exact to the minor unit and the same input always yields the same
// output on every machine. A half minor unit rounds up, which is the rule the invoice's own
// reconciliation applies to a credit, so the two agree to the cent.
func Prorate(amount int64, numer, denom int64) (int64, error) {
	if denom <= 0 {
		return 0, fmt.Errorf("money: proration needs a positive denominator, got %d", denom)
	}
	if numer < 0 {
		return 0, fmt.Errorf("money: proration needs a non-negative numerator, got %d", numer)
	}
	if numer > denom {
		return 0, fmt.Errorf("money: proration covers %d of %d days", numer, denom)
	}
	if amount == 0 || numer == 0 {
		return 0, nil
	}
	// amount*numer is checked before the addition below can overflow it further.
	product := amount * numer
	if product/numer != amount {
		return 0, ErrOverflow
	}
	half := denom / 2
	if product > 0 {
		if product > maxInt64-half {
			return 0, ErrOverflow
		}
		return (product + half) / denom, nil
	}
	if product < minInt64+half {
		return 0, ErrOverflow
	}
	return (product - half) / denom, nil
}

// ProrateAmount is Prorate over an amount, keeping its currency.
func ProrateAmount(amount Amount, numer, denom int64) (Amount, error) {
	if err := amount.Validate(); err != nil {
		return Amount{}, err
	}
	minor, err := Prorate(amount.Minor, numer, denom)
	if err != nil {
		return Amount{}, err
	}
	return Amount{Minor: minor, Currency: amount.Currency}, nil
}

// RoundToNearest returns total rounded to a multiple of step, half up, which is how a tax amount is
// brought to a whole minor unit the invoice can print.
func RoundToNearest(total int64, step int64) (int64, error) {
	if step <= 0 {
		return 0, fmt.Errorf("money: rounding needs a positive step, got %d", step)
	}
	rest := total % step
	if rest == 0 {
		return total, nil
	}
	if rest < 0 {
		return RoundToNearest(total-rest, step)
	}
	if rest*2 >= step {
		if total > maxInt64-(step-rest) {
			return 0, ErrOverflow
		}
		return total + (step - rest), nil
	}
	return total - rest, nil
}

// Sum adds amounts of one currency. An empty list has no currency and is refused rather than
// guessed at, and a list that mixes currencies is refused by the first pair that disagrees.
func Sum(amounts ...Amount) (Amount, error) {
	if len(amounts) == 0 {
		return Amount{}, fmt.Errorf("money: sum needs at least one amount")
	}
	total := Amount{Minor: 0, Currency: amounts[0].Currency}
	for _, amount := range amounts {
		next, err := total.Add(amount)
		if err != nil {
			return Amount{}, err
		}
		total = next
	}
	return total, nil
}

// SumShares is Sum over the shares of a split, which is how a test proves the shares add up.
func SumShares(shares []Amount) (Amount, error) {
	if len(shares) == 0 {
		return Amount{}, fmt.Errorf("money: sum needs at least one amount")
	}
	return Sum(shares...)
}

// Allocate divides total over the weights given, in proportion to each weight, and hands the
// remainder out one minor unit at a time in the order given. A zero-weight part receives nothing,
// and a total with no positive weight is refused.
func Allocate(total int64, weights []int64) ([]int64, error) {
	if len(weights) == 0 {
		return nil, fmt.Errorf("money: allocate needs at least one weight")
	}
	var sum int64
	for _, weight := range weights {
		if weight < 0 {
			return nil, fmt.Errorf("money: allocate needs non-negative weights, got %d", weight)
		}
		if weight > maxInt64-sum {
			return nil, ErrOverflow
		}
		sum += weight
	}
	if sum == 0 {
		return nil, fmt.Errorf("money: allocate needs a positive weight")
	}
	shares := make([]int64, len(weights))
	var handed int64
	for i, weight := range weights {
		if weight == 0 {
			continue
		}
		product := total * weight
		if weight != 0 && product/weight != total {
			return nil, ErrOverflow
		}
		shares[i] = product / sum
		handed += shares[i]
	}
	rest := total - handed
	step := int64(1)
	if rest < 0 {
		step = -1
		rest = -rest
	}
	for i := 0; rest > 0; i = (i + 1) % len(shares) {
		if weights[i] == 0 {
			continue
		}
		shares[i] += step
		rest--
	}
	return shares, nil
}

// maxInt64 is the largest int64, the ceiling every multiplication above is checked against.
const maxInt64 = 1<<63 - 1
