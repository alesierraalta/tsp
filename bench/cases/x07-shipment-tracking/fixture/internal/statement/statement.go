// Package statement assembles a partner's statement from the shipments of a period. Every amount is an
// integer number of minor units and every sum is checked; the fuel surcharge is allocated across the
// lines in proportion to their weight under a ceiling per line, and the tax the same way, so the lines
// add up to the statement exactly. A statement whose lines and total disagree is refused rather than
// written: a partner that reconciles the page against its own ledger must find the same numbers.
package statement

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"example.com/shiptrack/internal/clock"
	"example.com/shiptrack/internal/identity"
	"example.com/shiptrack/internal/money"
	"example.com/shiptrack/internal/textutil"
)

// Errors a close reports.
var (
	ErrInvalid = errors.New("statement: invalid statement")
	ErrTotals  = errors.New("statement: totals do not add up")
	ErrTooMany = errors.New("statement: too many lines")
	ErrPeriod  = errors.New("statement: period is not closed")
	ErrWindow  = errors.New("statement: period is outside the close window")
)

// maxLines is the hard ceiling of a statement, independent of configuration: a page that carries more
// lines than this is not a statement a partner reads.
const maxLines = 20000

// Item is one billable shipment as the close found it: what the shipment is, what it weighed and what
// the rate book charged for it.
type Item struct {
	Shipment string
	Carrier  string
	Service  string
	Lane     string
	Grams    int64
	Base     money.Amount
	Fuel     money.Amount
	At       time.Time
}

// Line is one line of a statement: the shipment, its freight, its allocated fuel and tax, and the
// total the partner pays for it.
type Line struct {
	Shipment string
	Carrier  string
	Service  string
	Lane     string
	Grams    int64
	At       time.Time
	Base     money.Amount
	Fuel     money.Amount
	Tax      money.Amount
	Total    money.Amount
}

// Statement is one partner's statement for one period.
type Statement struct {
	Tenant   string
	Partner  identity.Partner
	Period   clock.Period
	Lines    []Line
	Base     money.Amount
	Fuel     money.Amount
	Net      money.Amount
	Tax      money.Amount
	Total    money.Amount
	Currency string
	Built    time.Time
}

// Build assembles a statement. The items are sorted by shipment so the page is stable, the lines are
// capped, and the fuel and tax totals are allocated with the capped allocation rule: no line is
// charged more fuel than its freight, and the allocation refuses a total the ceilings cannot hold.
func Build(partner identity.Partner, period clock.Period, items []Item, taxBasisPoints int32, built time.Time) (Statement, error) {
	if !period.Valid() {
		return Statement{}, fmt.Errorf("%w: period %q", ErrInvalid, period)
	}
	if len(items) == 0 {
		return Statement{}, fmt.Errorf("%w: partner %q has no shipments in %s", ErrInvalid, partner.ID, period)
	}
	if len(items) > maxLines {
		return Statement{}, fmt.Errorf("%w: %d lines is above %d", ErrTooMany, len(items), maxLines)
	}
	ordered := append([]Item(nil), items...)
	sort.Slice(ordered, func(i, j int) bool {
		if !ordered[i].At.Equal(ordered[j].At) {
			return ordered[i].At.Before(ordered[j].At)
		}
		return ordered[i].Shipment < ordered[j].Shipment
	})
	lines := make([]Line, 0, len(ordered))
	weights := make([]int64, 0, len(ordered))
	ceilings := make([]money.Amount, 0, len(ordered))
	var base, fuel, net money.Amount
	for i, item := range ordered {
		if item.Shipment == "" || item.Carrier == "" || item.Grams <= 0 {
			return Statement{}, fmt.Errorf("%w: item %d", ErrInvalid, i)
		}
		if item.Base.Negative() || item.Fuel.Negative() {
			return Statement{}, fmt.Errorf("%w: item %d is negative", ErrInvalid, i)
		}
		baseSum, err := money.Add(base, item.Base)
		if err != nil {
			return Statement{}, fmt.Errorf("%w: %v", ErrTotals, err)
		}
		fuelSum, err := money.Add(fuel, item.Fuel)
		if err != nil {
			return Statement{}, fmt.Errorf("%w: %v", ErrTotals, err)
		}
		lineTotal, err := money.Add(item.Base, item.Fuel)
		if err != nil {
			return Statement{}, fmt.Errorf("%w: %v", ErrTotals, err)
		}
		netSum, err := money.Add(net, lineTotal)
		if err != nil {
			return Statement{}, fmt.Errorf("%w: %v", ErrTotals, err)
		}
		base, fuel, net = baseSum, fuelSum, netSum
		lines = append(lines, Line{
			Shipment: item.Shipment,
			Carrier:  item.Carrier,
			Service:  item.Service,
			Lane:     item.Lane,
			Grams:    item.Grams,
			At:       item.At,
			Base:     item.Base,
		})
		weights = append(weights, item.Grams)
		// A line can never be charged more tax than the freight it rides on: the ceiling is the line's own
		// freight, so a statement's tax is spread over the lines and never lands on one of them twice.
		ceilings = append(ceilings, lineTotal)
	}
	tax, err := money.ApplyBasisPoints(net, taxBasisPoints)
	if err != nil {
		return Statement{}, fmt.Errorf("%w: tax: %v", ErrTotals, err)
	}
	if tax > net {
		// The tax rate is validated at configuration, so this can only happen if the rate is applied to an
		// amount it does not fit; refuse rather than let a line's total include a tax above its freight.
		return Statement{}, fmt.Errorf("%w: tax %s is above the net %s", ErrTotals, tax, net)
	}
	shares, err := money.SplitCapped(tax, weights, ceilings)
	if err != nil {
		return Statement{}, fmt.Errorf("%w: tax allocation: %v", ErrTotals, err)
	}
	fuelShares, err := money.SplitCapped(fuel, weights, ceilings)
	if err != nil {
		return Statement{}, fmt.Errorf("%w: fuel allocation: %v", ErrTotals, err)
	}
	var totalTax, totalFuel, total money.Amount
	for i := range lines {
		lines[i].Tax = shares[i]
		lines[i].Fuel = fuelShares[i]
		lineTotal, err := money.Add(lines[i].Base, lines[i].Fuel)
		if err != nil {
			return Statement{}, fmt.Errorf("%w: %v", ErrTotals, err)
		}
		lineTotal, err = money.Add(lineTotal, lines[i].Tax)
		if err != nil {
			return Statement{}, fmt.Errorf("%w: %v", ErrTotals, err)
		}
		lines[i].Total = lineTotal
		totalTax, err = money.Add(totalTax, lines[i].Tax)
		if err != nil {
			return Statement{}, fmt.Errorf("%w: %v", ErrTotals, err)
		}
		totalFuel, err = money.Add(totalFuel, lines[i].Fuel)
		if err != nil {
			return Statement{}, fmt.Errorf("%w: %v", ErrTotals, err)
		}
		total, err = money.Add(total, lines[i].Total)
		if err != nil {
			return Statement{}, fmt.Errorf("%w: %v", ErrTotals, err)
		}
	}
	if totalFuel != fuel || totalTax != tax {
		return Statement{}, fmt.Errorf("%w: allocated fuel %s of %s, tax %s of %s", ErrTotals, totalFuel, fuel, totalTax, tax)
	}
	expected, err := money.Add(net, tax)
	if err != nil {
		return Statement{}, fmt.Errorf("%w: %v", ErrTotals, err)
	}
	if total != expected {
		return Statement{}, fmt.Errorf("%w: lines sum to %s, statement says %s", ErrTotals, total, expected)
	}
	return Statement{
		Tenant:   partner.Tenant,
		Partner:  partner,
		Period:   period,
		Lines:    lines,
		Base:     base,
		Fuel:     fuel,
		Net:      net,
		Tax:      tax,
		Total:    total,
		Currency: "EUR",
		Built:    built.UTC(),
	}, nil
}

// CheckCloseable refuses a period a close may not run for. The period must be valid, must be over by
// the clock, and must sit inside the deployment's window. The comparisons are on instants: a period is
// over when its end instant has passed, which is a UTC month boundary and not a local one.
func CheckCloseable(period clock.Period, now time.Time, windowMonths int) error {
	if !period.Valid() {
		return fmt.Errorf("%w: %q", ErrPeriod, period)
	}
	if !period.Closed(now) {
		return fmt.Errorf("%w: %s is still running", ErrPeriod, period)
	}
	if !clock.WithinWindow(period, now, windowMonths) {
		return fmt.Errorf("%w: %s is %d months back", ErrWindow, period, clock.PeriodOf(now).Distance(period))
	}
	return nil
}

// LineCount reports the statement's line count.
func (s Statement) LineCount() int { return len(s.Lines) }

// Summary renders the statement's totals for a page header.
func (s Statement) Summary() string {
	return fmt.Sprintf("%s %s: %d lines, net %s, tax %s, total %s",
		s.Partner.ID, s.Period, len(s.Lines), s.Net, s.Tax, s.Total)
}

// Describe renders one line for a console row.
func (l Line) Describe() string {
	return fmt.Sprintf("%s %s %s %s", l.Shipment, l.Service, textutil.Grams(l.Grams), l.Total)
}
