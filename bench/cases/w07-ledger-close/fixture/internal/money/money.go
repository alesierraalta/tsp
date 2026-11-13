// Package money holds the ledger's amounts. An amount is an integer number of minor units (cents)
// and it stays one: no path in this package converts through float64, so a sum of 0.10 and 0.20 is
// 0.30 and a rate is applied by integer division with an explicit rounding rule instead of by a
// binary fraction.
package money

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Errors the arithmetic reports: an amount the ledger cannot represent is refused, never clamped.
var (
	ErrInvalid  = errors.New("money: invalid amount")
	ErrOverflow = errors.New("money: amount out of range")
	ErrRounding = errors.New("money: cannot round")
)

// Amount is an amount in minor units.
type Amount int64

// MinorPerUnit is the number of minor units in one currency unit.
const MinorPerUnit = 100

// Parse reads a decimal amount into minor units: "1.15" is 115, "45" is 4500, "-0.05" is -5. A value with
// more than two fraction digits is refused rather than rounded, and so is a value with a thousands
// separator, an exponent or a stray space: the ledger accepts the exact form it writes.
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
		return 0, fmt.Errorf("%w: %q", ErrInvalid, raw)
	}
	minor, err := strconv.ParseInt(fraction, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %q", ErrInvalid, raw)
	}
	// Both steps are checked in the same style as the arithmetic: scaling by 100 and adding the minor
	// units wrap rather than fail, and the most negative amount is one the type can hold, so the sign
	// is carried into the whole part before the bound is applied.
	scaled, ok := mulChecked(units, MinorPerUnit)
	if !ok {
		return 0, fmt.Errorf("%w: %q", ErrOverflow, raw)
	}
	if negative {
		minor = -minor
	}
	amount, ok := addChecked(scaled, minor)
	if !ok {
		return 0, fmt.Errorf("%w: %q", ErrOverflow, raw)
	}
	return Amount(amount), nil
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
	// The magnitude is taken in uint64 so the most negative amount is rendered as well.
	magnitude := uint64(value)
	if value < 0 {
		magnitude = uint64(-(value + 1)) + 1
	}
	return sign + strconv.FormatUint(magnitude/MinorPerUnit, 10) + "." +
		fmt.Sprintf("%02d", magnitude%MinorPerUnit)
}

// Negative reports whether the amount is below zero.
func (a Amount) Negative() bool { return a < 0 }

// Add sums two amounts and refuses a result the type cannot hold.
func Add(a, b Amount) (Amount, error) {
	sum, ok := addChecked(int64(a), int64(b))
	if !ok {
		return 0, fmt.Errorf("%w: %s + %s", ErrOverflow, a, b)
	}
	return Amount(sum), nil
}

// Mul multiplies an amount by a unit count and refuses an overflow.
func Mul(a Amount, units int64) (Amount, error) {
	product, ok := mulChecked(int64(a), units)
	if !ok {
		return 0, fmt.Errorf("%w: %s x %d", ErrOverflow, a, units)
	}
	return Amount(product), nil
}

// RoundRatio applies numerator/denominator to an amount and rounds half to even: 2.5 becomes 2 and
// 3.5 becomes 4, so a value that lands exactly between two minor units is not steered upward by
// every compensation line. Both steps are checked.
func RoundRatio(amount Amount, numerator, denominator int64) (Amount, error) {
	if denominator <= 0 {
		return 0, fmt.Errorf("%w: denominator %d", ErrRounding, denominator)
	}
	product, ok := mulChecked(int64(amount), numerator)
	if !ok {
		return 0, fmt.Errorf("%w: %s x %d", ErrOverflow, amount, numerator)
	}
	rounded, err := roundHalfEven(product, denominator)
	if err != nil {
		return 0, err
	}
	return Amount(rounded), nil
}

// addChecked adds two int64 values and reports whether the result fits.
func addChecked(a, b int64) (int64, bool) {
	if b > 0 && a > math.MaxInt64-b {
		return 0, false
	}
	if b < 0 && a < math.MinInt64-b {
		return 0, false
	}
	return a + b, true
}

// mulChecked multiplies two int64 values and reports whether the result fits. MinInt64 x -1 wraps
// to MinInt64 and divides back to itself, so it is the one product the round-trip test cannot see.
func mulChecked(a, b int64) (int64, bool) {
	if a == 0 || b == 0 {
		return 0, true
	}
	if (a == math.MinInt64 && b == -1) || (b == math.MinInt64 && a == -1) {
		return 0, false
	}
	product := a * b
	// A wrapped product cannot divide back to the factor: the wrap moves it by a multiple of 2^64 and
	// truncating division moves it by less than |b|.
	if product/b != a {
		return 0, false
	}
	return product, true
}

// roundHalfEven divides numerator by denominator, rounding half to even. The magnitude is taken in
// uint64 so the most negative numerator is rounded rather than overflowed, and the half comparison
// is made on magnitudes so doubling cannot overflow.
func roundHalfEven(numerator, denominator int64) (int64, error) {
	if denominator <= 0 {
		return 0, fmt.Errorf("%w: denominator %d", ErrRounding, denominator)
	}
	negative := numerator < 0
	magnitude := uint64(numerator)
	if negative {
		magnitude = uint64(-(numerator + 1)) + 1
	}
	divisor := uint64(denominator)
	quotient := magnitude / divisor
	remainder := magnitude % divisor
	if twice := remainder * 2; twice > divisor || (twice == divisor && quotient%2 == 1) {
		quotient++
	}
	if quotient > math.MaxInt64 {
		return 0, fmt.Errorf("%w: quotient does not fit", ErrOverflow)
	}
	if negative {
		return -int64(quotient), nil
	}
	return int64(quotient), nil
}
