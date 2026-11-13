package store

import (
	"fmt"
	"strconv"
	"strings"
)

// Cents is an amount held as integer cents. The ledger has no fractional amounts, so a value that
// carries a fraction is refused rather than rounded into one.
type Cents int64

// ParseCents reads a decimal amount into cents: "1.15" is 115 and "45.00" is 4500.
func ParseCents(raw string) (Cents, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("store: empty amount")
	}
	negative := false
	switch raw[0] {
	case '-':
		negative = true
		raw = raw[1:]
	case '+':
		raw = raw[1:]
	}
	whole, fraction, ok := strings.Cut(raw, ".")
	if !ok {
		fraction = "00"
	}
	if whole == "" {
		whole = "0"
	}
	if len(fraction) > 2 {
		return 0, fmt.Errorf("store: amount %q carries more than two fraction digits", raw)
	}
	for len(fraction) < 2 {
		fraction += "0"
	}
	units, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("store: amount %q", whole)
	}
	hundredths, err := strconv.ParseInt(fraction, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("store: amount %q", fraction)
	}
	cents := Cents(units*100 + hundredths)
	if negative {
		cents = -cents
	}
	return cents, nil
}

// String renders cents as a decimal amount with two fraction digits.
func (c Cents) String() string {
	sign := ""
	value := int64(c)
	if value < 0 {
		sign = "-"
		value = -value
	}
	return fmt.Sprintf("%s%d.%02d", sign, value/100, value%100)
}

// Add sums cents without leaving the integer domain.
func Add(amounts ...Cents) Cents {
	var total Cents
	for _, amount := range amounts {
		total += amount
	}
	return total
}
