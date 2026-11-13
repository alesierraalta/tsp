// Package usage holds the metered events the ledger bills for: their wire form, their field rules
// and the two decoders an ingest request may use.
package usage

import (
	"errors"
	"fmt"
	"regexp"
	"time"

	"example.com/ledgerclose/internal/identity"
	"example.com/ledgerclose/internal/money"
)

// Errors the event rules report.
var (
	ErrInvalid   = errors.New("usage: invalid event")
	ErrTooLarge  = errors.New("usage: body exceeds the bound")
	ErrDuplicate = errors.New("usage: duplicate event id")
	ErrMalformed = errors.New("usage: malformed batch")
)

// Metrics are the metered quantities the rate book prices.
var Metrics = []string{"api_call", "storage_gb", "seat", "message"}

// eventIDRE is the event id rule. The id is written into the journal, the CSV export and the
// statement, so it is lowercase kebab and short.
var eventIDRE = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{7,63}$`)

// Event is one metered event. Amounts are integer minor units on the wire as well, so a client and
// the ledger never disagree about what 0.1 means.
type Event struct {
	ID         string       `json:"id"`
	Account    string       `json:"account"`
	Metric     string       `json:"metric"`
	Quantity   int64        `json:"quantity"`
	UnitAmount money.Amount `json:"unit_amount"`
	Currency   string       `json:"currency"`
	OccurredAt time.Time    `json:"occurred_at"`
}

// Limits are the caps an ingest, a statement and a page apply. They live with the events because
// they bound an event's fields, and the configuration builds one value from its own settings so no
// handler compares against a raw setting.
type Limits struct {
	PageSize         int
	MaxBatch         int
	MaxBody          int64
	MaxEventQuantity int64
	MaxUnitAmount    money.Amount
	MaxLines         int
	MaxResponse      int64
	TaxBasisPoints   int64
}

// Valid reports whether the caps can be applied at all. The zero Limits is not valid: a service
// started from a zero value would refuse every request, and refusing at startup says so once.
func (l Limits) Valid() bool {
	return l.PageSize > 0 && l.MaxBatch > 0 && l.MaxBody > 0 && l.MaxLines > 0 && l.MaxResponse > 0
}

// ClampLines bounds the line count of a statement and reports whether the body fits. A statement is
// assembled from every event of an account, so an account above the cap fails the close instead of
// having its statement truncated.
func (l Limits) ClampLines(want int) (int, bool) {
	if want > l.MaxLines {
		return l.MaxLines, false
	}
	return want, true
}

// ValidMetric reports whether the ledger prices a metric.
func ValidMetric(metric string) bool {
	for _, known := range Metrics {
		if known == metric {
			return true
		}
	}
	return false
}

// MaxEventAge and MaxClockSkew bound the instant an event may claim.
const (
	MaxEventAge  = 13 * 30 * 24 * time.Hour
	MaxClockSkew = 5 * time.Minute
)

// Validate applies the field rules the decoder cannot express: forms, caps, currency and the timestamp
// window. The account is checked by CheckAccount, which needs the directory and the caller's tenant.
func (e Event) Validate(limits Limits, now time.Time) error {
	switch {
	case !eventIDRE.MatchString(e.ID):
		return fmt.Errorf("%w: id %q", ErrInvalid, e.ID)
	case e.Account == "":
		return fmt.Errorf("%w: id %q has no account", ErrInvalid, e.ID)
	case !ValidMetric(e.Metric):
		return fmt.Errorf("%w: metric %q", ErrInvalid, e.Metric)
	case e.Quantity <= 0 || e.Quantity > limits.MaxEventQuantity:
		return fmt.Errorf("%w: quantity %d is not in 1..%d", ErrInvalid, e.Quantity, limits.MaxEventQuantity)
	case e.UnitAmount <= 0 || e.UnitAmount > limits.MaxUnitAmount:
		return fmt.Errorf("%w: unit amount %s is not in 0.01..%s", ErrInvalid, e.UnitAmount, limits.MaxUnitAmount)
	case e.Currency == "" || len(e.Currency) > 8:
		return fmt.Errorf("%w: currency %q", ErrInvalid, e.Currency)
	case e.OccurredAt.IsZero():
		return fmt.Errorf("%w: id %q has no timestamp", ErrInvalid, e.ID)
	}
	occurred := e.OccurredAt.UTC()
	if occurred.After(now.Add(MaxClockSkew)) {
		return fmt.Errorf("%w: id %q is dated %s, after now", ErrInvalid, e.ID, occurred.Format(time.RFC3339))
	}
	if occurred.Before(now.Add(-MaxEventAge)) {
		return fmt.Errorf("%w: id %q is dated %s, more than %v ago", ErrInvalid, e.ID,
			occurred.Format(time.RFC3339), MaxEventAge)
	}
	return nil
}

// CheckAccount ties an event to the account it claims and to the caller's tenant. The account comes
// from the directory, so an event cannot name one the deployment does not hold; the tenant is the
// token's, so an account of another tenant is refused rather than billed.
func CheckAccount(event Event, account identity.Account, tenant string) error {
	if account.ID != event.Account {
		return fmt.Errorf("%w: id %q names %q, resolved %q", ErrInvalid, event.ID, event.Account, account.ID)
	}
	if account.Tenant != tenant {
		return fmt.Errorf("%w: id %q names an account of another tenant", ErrInvalid, event.ID)
	}
	if account.Currency != event.Currency {
		return fmt.Errorf("%w: id %q is in %s, account bills in %s", ErrInvalid, event.ID, event.Currency, account.Currency)
	}
	return nil
}

// Gross is the event's charge before the rate is applied: quantity times unit amount, checked.
func (e Event) Gross() (money.Amount, error) {
	gross, err := money.Mul(e.UnitAmount, e.Quantity)
	if err != nil {
		return 0, fmt.Errorf("%w: id %q: %v", ErrInvalid, e.ID, err)
	}
	return gross, nil
}
