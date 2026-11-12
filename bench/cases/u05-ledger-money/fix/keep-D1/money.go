// Package ledger holds settlement money as integer cents.
package ledger

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ErrFormat reports an amount the ledger cannot read.
var ErrFormat = errors.New("ledger: malformed amount")

// ErrParts reports a split into fewer than one share.
var ErrParts = errors.New("ledger: parts must be positive")

// ParseCents reads a decimal amount and returns it in cents. Two fraction digits are exact; a third digit is
// refused rather than rounded, so an amount the ledger cannot hold never enters a posting.
func ParseCents(decimal string) (int64, error) {
	text := strings.TrimSpace(decimal)
	if text == "" {
		return 0, fmt.Errorf("%w: empty amount", ErrFormat)
	}
	if _, fraction, ok := strings.Cut(text, "."); ok && len(fraction) > 2 {
		return 0, fmt.Errorf("%w: %q carries more than two fraction digits", ErrFormat, decimal)
	}
	units, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %q", ErrFormat, decimal)
	}
	return int64(units * 100), nil
}

// FormatCents writes an amount as a decimal string with two fraction digits.
func FormatCents(cents int64) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	return fmt.Sprintf("%s%d.%02d", sign, cents/100, cents%100)
}

// Split divides total into parts shares whose sum is total, differing by at most one cent.
func Split(total int64, parts int) ([]int64, error) {
	if parts < 1 {
		return nil, ErrParts
	}
	share := total / int64(parts)
	remainder := total % int64(parts)
	step := int64(1)
	if remainder < 0 {
		step = -1
	}
	shares := make([]int64, parts)
	for i := range shares {
		shares[i] = share
	}
	for i := int64(0); i < remainder*step; i++ {
		shares[i] += step
	}
	return shares, nil
}

// ApplyRate applies a basis-point rate and rounds half away from zero.
func ApplyRate(cents int64, bps int64) int64 {
	product := cents * bps
	half := int64(10_000) / 2
	if product < 0 {
		return -((-product + half) / 10_000)
	}
	return (product + half) / 10_000
}

// Sum adds amounts.
func Sum(amounts []int64) int64 {
	var total int64
	for _, amount := range amounts {
		total += amount
	}
	return total
}

// Entry is one posting of a journal.
type Entry struct {
	Account string
	Cents   int64
	Memo    string
}

// Account is a journal of postings with a running balance.
type Account struct {
	entries []Entry
}

// Post appends a posting and returns the new balance.
func (a *Account) Post(entry Entry) int64 {
	a.entries = append(a.entries, entry)
	return a.Balance()
}

// Balance is the net amount of the journal.
func (a *Account) Balance() int64 {
	var total int64
	for _, entry := range a.entries {
		total += entry.Cents
	}
	return total
}

// Entries returns detached copies of the journal.
func (a *Account) Entries() []Entry {
	out := make([]Entry, len(a.entries))
	copy(out, a.entries)
	return out
}

// Reconcile reports whether the journal sums to the expected amount.
func (a *Account) Reconcile(expected int64) error {
	balance := a.Balance()
	if balance != expected {
		return fmt.Errorf("ledger: balance %s, expected %s", FormatCents(balance), FormatCents(expected))
	}
	return nil
}
