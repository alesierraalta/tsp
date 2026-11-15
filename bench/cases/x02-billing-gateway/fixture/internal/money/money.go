// Package money is the billing service's money type: a count of minor units and the currency it is
// counted in. There is no float in this package and no float in a stored amount, because a binary
// fraction cannot hold a tenth of a cent and the error compounds once a million invoices are summed.
package money

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Errors an amount reports.
var (
	// ErrCurrency is returned for a currency that is not three ASCII letters.
	ErrCurrency = errors.New("money: a currency is three letters")
	// ErrMismatch is returned when two amounts of different currencies meet.
	ErrMismatch = errors.New("money: the amounts are in different currencies")
	// ErrOverflow is returned when an operation would leave the range an int64 holds.
	ErrOverflow = errors.New("money: the amount overflows")
	// ErrNegative is returned where an amount must not be negative.
	ErrNegative = errors.New("money: a negative amount is not allowed here")
)

// Amount is a quantity of money in the minor unit of its currency: cents for EUR and USD, yen for
// JPY. Zero is the zero amount of no currency, which a caller must not store.
type Amount struct {
	Minor    int64  `json:"minor"`
	Currency string `json:"currency"`
}

// New returns an amount of minor units in a currency. The currency is upper-cased and checked, so a
// deployment that reads "eur" from the environment stores "EUR" and two amounts of the same money
// compare equal whatever case the operator typed.
func New(minor int64, currency string) (Amount, error) {
	code := strings.ToUpper(strings.TrimSpace(currency))
	if !validCurrency(code) {
		return Amount{}, fmt.Errorf("%w: %q", ErrCurrency, currency)
	}
	return Amount{Minor: minor, Currency: code}, nil
}

// Zero returns the zero amount of a currency.
func Zero(currency string) Amount {
	amount, err := New(0, currency)
	if err != nil {
		return Amount{}
	}
	return amount
}

// validCurrency reports whether code is three ASCII letters.
func validCurrency(code string) bool {
	if len(code) != 3 {
		return false
	}
	for i := 0; i < len(code); i++ {
		c := code[i]
		if c < 'A' || c > 'Z' {
			return false
		}
	}
	return true
}

// Validate reports whether the amount is one a deployment may store.
func (a Amount) Validate() error {
	if !validCurrency(a.Currency) {
		return fmt.Errorf("%w: %q", ErrCurrency, a.Currency)
	}
	return nil
}

// IsZero reports whether the amount is zero of some currency.
func (a Amount) IsZero() bool { return a.Minor == 0 && a.Currency != "" }

// Add returns the sum of two amounts of the same currency.
func (a Amount) Add(b Amount) (Amount, error) {
	if a.Currency != b.Currency {
		return Amount{}, fmt.Errorf("%w: %s and %s", ErrMismatch, a.Currency, b.Currency)
	}
	sum := a.Minor + b.Minor
	if (b.Minor > 0 && sum < a.Minor) || (b.Minor < 0 && sum > a.Minor) {
		return Amount{}, ErrOverflow
	}
	return Amount{Minor: sum, Currency: a.Currency}, nil
}

// Sub returns the difference of two amounts of the same currency.
func (a Amount) Sub(b Amount) (Amount, error) {
	negated := Amount{Minor: -b.Minor, Currency: b.Currency}
	if b.Minor == minInt64 {
		return Amount{}, ErrOverflow
	}
	return a.Add(negated)
}

// Mul returns the amount scaled by a whole number of times.
func (a Amount) Mul(times int64) (Amount, error) {
	if times == 0 || a.Minor == 0 {
		return Amount{Minor: 0, Currency: a.Currency}, nil
	}
	product := a.Minor * times
	if product/times != a.Minor {
		return Amount{}, ErrOverflow
	}
	return Amount{Minor: product, Currency: a.Currency}, nil
}

// Neg returns the amount with the opposite sign, which is how a credit is written down.
func (a Amount) Neg() (Amount, error) {
	if a.Minor == minInt64 {
		return Amount{}, ErrOverflow
	}
	return Amount{Minor: -a.Minor, Currency: a.Currency}, nil
}

// Abs returns the amount without its sign.
func (a Amount) Abs() (Amount, error) {
	if a.Minor < 0 {
		return a.Neg()
	}
	return a, nil
}

// NonNegative returns the amount, or ErrNegative when it carries a negative count.
func (a Amount) NonNegative() (Amount, error) {
	if a.Minor < 0 {
		return Amount{}, fmt.Errorf("%w: %s", ErrNegative, a)
	}
	return a, nil
}

// Cmp compares two amounts: -1, 0 or 1. Amounts of different currencies cannot be compared and the
// caller gets the mismatch as an error rather than a silently wrong ordering.
func (a Amount) Cmp(b Amount) (int, error) {
	if a.Currency != b.Currency {
		return 0, fmt.Errorf("%w: %s and %s", ErrMismatch, a.Currency, b.Currency)
	}
	switch {
	case a.Minor < b.Minor:
		return -1, nil
	case a.Minor > b.Minor:
		return 1, nil
	}
	return 0, nil
}

// String renders the amount the way a customer reads it: the major unit, a dot and the minor digits.
func (a Amount) String() string {
	return formatMinor(a.Minor, a.Currency)
}

// MarshalJSON writes an amount as an object, so a reader can never mistake minor units for major ones.
func (a Amount) MarshalJSON() ([]byte, error) {
	if err := a.Validate(); err != nil {
		return nil, err
	}
	type wire struct {
		Minor    int64  `json:"minor"`
		Currency string `json:"currency"`
	}
	return json.Marshal(wire{Minor: a.Minor, Currency: a.Currency})
}

// UnmarshalJSON reads the object MarshalJSON writes. A bare number is refused: the point of the
// encoding is that the unit travels with the value.
func (a *Amount) UnmarshalJSON(data []byte) error {
	var wire struct {
		Minor    int64  `json:"minor"`
		Currency string `json:"currency"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return fmt.Errorf("money: an amount is an object with minor and currency: %w", err)
	}
	parsed, err := New(wire.Minor, wire.Currency)
	if err != nil {
		return err
	}
	*a = parsed
	return nil
}

// minInt64 is the smallest int64, the one value whose negation does not fit.
const minInt64 = -1 << 63
