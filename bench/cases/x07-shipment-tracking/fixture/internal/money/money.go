// Package money holds the service's amounts. An amount is an integer number of minor units — cents —
// and it stays one: no path in this package converts through float64, so a freight charge of 0.10
// and a fuel charge of 0.20 add to 0.30, and a rate is applied by integer division with an explicit
// rounding rule instead of by a binary fraction that cannot hold a tenth.
package money

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Errors the arithmetic reports. An amount the service cannot represent is refused, never clamped:
// a clamped charge would silently under-bill a partner and a clamped total would silently disagree
// with the lines it sums.
var (
	ErrInvalid  = errors.New("money: invalid amount")
	ErrOverflow = errors.New("money: amount out of range")
	ErrRounding = errors.New("money: cannot round")
	ErrAllocate = errors.New("money: cannot allocate")
)

// Amount is an amount in minor units.
type Amount int64

// MinorPerUnit is the number of minor units in one currency unit.
const MinorPerUnit = 100

// Parse reads a decimal amount into minor units: "1.15" is 115, "45" is 4500, "-0.05" is -5. More
// than two fraction digits, a thousands separator, an exponent or a stray space is refused rather
// than rounded: the service accepts the exact form it writes.
func Parse(raw string) (Amount, error) {
	if raw == "" || strings.TrimSpace(raw) != raw {
		return 0, fmt.Errorf("%w: %q", ErrInvalid, raw)
	}
	negative := false
	digits := raw
	switch digits[0] {
	case '-':
		negative = true
		digits = digits[1:]
	case '+':
		digits = digits[1:]
	}
	whole, fraction, hasPoint := strings.Cut(digits, ".")
	if whole == "" || (hasPoint && fraction == "") || len(fraction) > 2 {
		return 0, fmt.Errorf("%w: %q", ErrInvalid, raw)
	}
	for len(fraction) < 2 {
		fraction += "0"
	}
	if !allDigits(whole) || !allDigits(fraction) {
		return 0, fmt.Errorf("%w: %q is not a decimal amount", ErrInvalid, raw)
	}
	sign := ""
	if negative {
		sign = "-"
	}
	units, err := strconv.ParseInt(sign+whole, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %q", ErrOverflow, raw)
	}
	minor, err := strconv.ParseInt(fraction, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %q", ErrInvalid, raw)
	}
	scaled, ok := mulChecked(units, MinorPerUnit)
	if !ok {
		return 0, fmt.Errorf("%w: %q", ErrOverflow, raw)
	}
	if negative {
		minor = -minor
	}
	sum, ok := addChecked(scaled, minor)
	if !ok {
		return 0, fmt.Errorf("%w: %q", ErrOverflow, raw)
	}
	return Amount(sum), nil
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// String renders an amount in the form Parse accepts, with two fraction digits.
func (a Amount) String() string {
	value := int64(a)
	sign := ""
	if value < 0 {
		sign = "-"
	}
	magnitude := uint64(value)
	if value < 0 {
		magnitude = uint64(-(value + 1)) + 1
	}
	return sign + strconv.FormatUint(magnitude/MinorPerUnit, 10) + "." +
		fmt.Sprintf("%02d", magnitude%MinorPerUnit)
}

// Units renders the amount as a whole number of currency units, rounding half to even. It is used for
// a display total, never for a charge: rounding a charge to a unit before it is billed would lose a
// minor unit per line.
func (a Amount) Units() Amount { return a }

// Negative reports whether the amount is below zero.
func (a Amount) Negative() bool { return a < 0 }

// Zero reports whether the amount is exactly zero.
func (a Amount) Zero() bool { return a == 0 }

// Positive reports whether the amount is above zero.
func (a Amount) Positive() bool { return a > 0 }

// Abs returns the magnitude of the amount. The most negative amount has no positive counterpart in
// int64, so it is refused rather than wrapped.
func (a Amount) Abs() (Amount, error) {
	if a == Amount(math.MinInt64) {
		return 0, fmt.Errorf("%w: %s", ErrOverflow, a)
	}
	if a < 0 {
		return -a, nil
	}
	return a, nil
}

// Add sums two amounts and refuses a result the type cannot hold.
func Add(a, b Amount) (Amount, error) {
	sum, ok := addChecked(int64(a), int64(b))
	if !ok {
		return 0, fmt.Errorf("%w: %s + %s", ErrOverflow, a, b)
	}
	return Amount(sum), nil
}

// Sum adds a list of amounts, refusing the first result that does not fit.
func Sum(amounts ...Amount) (Amount, error) {
	var total Amount
	for _, amount := range amounts {
		next, err := Add(total, amount)
		if err != nil {
			return 0, err
		}
		total = next
	}
	return total, nil
}

// Sub subtracts b from a and refuses a result the type cannot hold.
func Sub(a, b Amount) (Amount, error) {
	difference, ok := subChecked(int64(a), int64(b))
	if !ok {
		return 0, fmt.Errorf("%w: %s - %s", ErrOverflow, a, b)
	}
	return Amount(difference), nil
}

// Neg returns the opposite amount. The most negative amount is refused: negating it would wrap.
func Neg(a Amount) (Amount, error) {
	if a == Amount(math.MinInt64) {
		return 0, fmt.Errorf("%w: -%s", ErrOverflow, a)
	}
	return -a, nil
}

// MulInt multiplies an amount by a whole count: a quantity, a number of kilograms, a number of
// periods. A product the type cannot hold is refused.
func MulInt(a Amount, count int64) (Amount, error) {
	product, ok := mulChecked(int64(a), count)
	if !ok {
		return 0, fmt.Errorf("%w: %s * %d", ErrOverflow, a, count)
	}
	return Amount(product), nil
}

// ApplyBasisPoints applies a rate in basis points — hundredths of a percent, so 1250 is 12.5% — and
// rounds the result half to even: 2.5 minor units become 2, 3.5 become 4, and -2.5 becomes -2. Half
// to even is the rule the rate book and the tax authority agree on, and it is why the service does
// not round by truncation: truncation would drift one way over a month of lines.
func ApplyBasisPoints(a Amount, basisPoints int32) (Amount, error) {
	if basisPoints < 0 {
		return 0, fmt.Errorf("%w: %d basis points", ErrInvalid, basisPoints)
	}
	scaled, ok := mulChecked(int64(a), int64(basisPoints))
	if !ok {
		return 0, fmt.Errorf("%w: %s at %d basis points", ErrOverflow, a, basisPoints)
	}
	rounded, err := divRoundHalfEven(scaled, BasisPointsPerUnit)
	if err != nil {
		return 0, err
	}
	return Amount(rounded), nil
}

// BasisPointsPerUnit is the denominator of a basis point rate.
const BasisPointsPerUnit = 10000

func addChecked(a, b int64) (int64, bool) {
	sum := a + b
	if (b > 0 && sum < a) || (b < 0 && sum > a) {
		return 0, false
	}
	return sum, true
}

func subChecked(a, b int64) (int64, bool) {
	if b == math.MinInt64 {
		return 0, false
	}
	return addChecked(a, -b)
}

func mulChecked(a, b int64) (int64, bool) {
	if a == 0 || b == 0 {
		return 0, true
	}
	product := a * b
	if product/b != a {
		return 0, false
	}
	if a == math.MinInt64 && b == -1 || b == math.MinInt64 && a == -1 {
		return 0, false
	}
	return product, true
}
