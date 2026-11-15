// Package invoice assembles the statement of one account for one period: its lines, the fee each line
// carries and the totals the statement prints.
package invoice

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"example.com/ledgerclose/internal/calendar"
	"example.com/ledgerclose/internal/identity"
	"example.com/ledgerclose/internal/money"
	"example.com/ledgerclose/internal/pricing"
	"example.com/ledgerclose/internal/usage"
)

// Errors the assembly reports.
var (
	ErrEmpty   = errors.New("invoice: no usage for the account")
	ErrInvalid = errors.New("invoice: invalid assembly")
	ErrTooMany = errors.New("invoice: more lines than the cap")
	ErrPeriod  = errors.New("invoice: event outside the period")
)

// Line is one billed event.
type Line struct {
	EventID    string       `json:"event_id"`
	Metric     string       `json:"metric"`
	Quantity   int64        `json:"quantity"`
	UnitAmount money.Amount `json:"unit_amount"`
	Gross      money.Amount `json:"gross"`
	Fee        money.Amount `json:"fee"`
	OccurredAt time.Time    `json:"occurred_at"`
}

// Invoice is one account's statement for one period.
type Invoice struct {
	ID          string          `json:"id"`
	Tenant      string          `json:"tenant"`
	Account     string          `json:"account"`
	AccountName string          `json:"account_name"`
	Currency    string          `json:"currency"`
	Period      calendar.Period `json:"period"`
	Lines       []Line          `json:"lines"`
	Net         money.Amount    `json:"net"`
	Tax         money.Amount    `json:"tax"`
	Total       money.Amount    `json:"total"`
	Issued      time.Time       `json:"issued"`
}

// ID is the statement id of an account and period. Every part is a validated segment, so the id is a key
// and not a path.
func ID(tenant string, period calendar.Period, account string) string {
	return "inv-" + tenant + "-" + account + "-" + period.String()
}

// LineCount is the number of lines the statement carries.
func (inv Invoice) LineCount() int { return len(inv.Lines) }

// Assemble builds one account's statement.
//
// The inputs are checked before any arithmetic: the account's own rules, the account against the caller's
// tenant, every event against the period, and the line count against the configured cap. A statement is
// never truncated — an account with more events than the cap fails the close, so an operator raises the
// cap rather than billing half a month.
func Assemble(account identity.Account, period calendar.Period, events []usage.Event, book *pricing.Book,
	limits usage.Limits, now time.Time) (Invoice, error) {
	if err := account.Validate(); err != nil {
		return Invoice{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if !period.Valid() {
		return Invoice{}, fmt.Errorf("%w: period %q", ErrInvalid, period)
	}
	if len(events) == 0 {
		return Invoice{}, fmt.Errorf("%w: account %s period %s", ErrEmpty, account.ID, period)
	}
	if _, within := limits.ClampLines(len(events)); !within {
		return Invoice{}, fmt.Errorf("%w: %d events, cap %d", ErrTooMany, len(events), limits.MaxLines)
	}
	sort.Slice(events, func(i, j int) bool {
		if events[i].OccurredAt.Equal(events[j].OccurredAt) {
			return events[i].ID < events[j].ID
		}
		return events[i].OccurredAt.Before(events[j].OccurredAt)
	})

	// The band a metric's fee uses is decided by the month's quantity, so the quantities are summed first:
	// a per-event walk would price the first event in one band and the last in another.
	quantities := make(map[string]int64, len(usage.Metrics))
	for _, event := range events {
		if err := event.Validate(limits, now); err != nil {
			return Invoice{}, fmt.Errorf("%w: %v", ErrInvalid, err)
		}
		if err := usage.CheckAccount(event, account, account.Tenant); err != nil {
			return Invoice{}, fmt.Errorf("%w: %v", ErrInvalid, err)
		}
		if !period.Contains(event.OccurredAt.UTC()) {
			return Invoice{}, fmt.Errorf("%w: event %s is dated %s, not %s", ErrPeriod, event.ID,
				event.OccurredAt.UTC().Format(time.RFC3339), period)
		}
		sum, ok := addCount(quantities[event.Metric], event.Quantity)
		if !ok {
			return Invoice{}, fmt.Errorf("%w: quantity of %s overflows", ErrInvalid, event.Metric)
		}
		quantities[event.Metric] = sum
	}
	ratios := make(map[string]int64, len(quantities))
	for metric, quantity := range quantities {
		_, ratio, err := book.Band(account.RateTable, metric, quantity)
		if err != nil {
			return Invoice{}, fmt.Errorf("%w: %v", ErrInvalid, err)
		}
		ratios[metric] = ratio
	}

	lines := make([]Line, 0, len(events))
	fees := money.Amount(0)
	for _, event := range events {
		gross, err := event.Gross()
		if err != nil {
			return Invoice{}, fmt.Errorf("%w: %v", ErrInvalid, err)
		}
		ratio, ok := ratios[event.Metric]
		if !ok {
			return Invoice{}, fmt.Errorf("%w: no band for %s", ErrInvalid, event.Metric)
		}
		fee, err := money.RoundRatio(gross, ratio, 10_000)
		if err != nil {
			return Invoice{}, fmt.Errorf("%w: fee of %s: %v", ErrInvalid, event.ID, err)
		}
		lines = append(lines, Line{
			EventID:    event.ID,
			Metric:     event.Metric,
			Quantity:   event.Quantity,
			UnitAmount: event.UnitAmount,
			Gross:      gross,
			Fee:        fee,
			OccurredAt: event.OccurredAt.UTC(),
		})
		if fees, err = money.Add(fees, fee); err != nil {
			return Invoice{}, fmt.Errorf("%w: net: %v", ErrInvalid, err)
		}
	}
	net := fees
	if net.Negative() {
		return Invoice{}, fmt.Errorf("%w: net %s is negative", ErrInvalid, net)
	}
	tax, err := money.RoundRatio(net, limits.TaxBasisPoints, 10_000)
	if err != nil {
		return Invoice{}, fmt.Errorf("%w: tax: %v", ErrInvalid, err)
	}
	total, err := money.Add(net, tax)
	if err != nil {
		return Invoice{}, fmt.Errorf("%w: total: %v", ErrInvalid, err)
	}
	return Invoice{
		ID:          ID(account.Tenant, period, account.ID),
		Tenant:      account.Tenant,
		Account:     account.ID,
		AccountName: account.DisplayName,
		Currency:    account.Currency,
		Period:      period,
		Lines:       lines,
		Net:         net,
		Tax:         tax,
		Total:       total,
		Issued:      now.UTC(),
	}, nil
}

// addCount adds two counts without wrapping. Quantities are counts and not amounts, and a negative one is
// not a quantity, so both sides must be non-negative.
func addCount(a, b int64) (int64, bool) {
	if a < 0 || b < 0 || a > math.MaxInt64-b {
		return 0, false
	}
	return a + b, true
}
