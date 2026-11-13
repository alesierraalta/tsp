// Package tax applies a region's rate to a taxable base. A rate is an integer number of basis points,
// so 19% is 1900 at a basis of 10000 and no rate is ever a float that renders as 1899.9999.
package tax

import (
	"fmt"
	"sort"
	"strings"

	"example.com/billing/internal/money"
)

// Rate is one region's tax rate.
type Rate struct {
	// Region is the code a subscription declares, such as "ES" or "DE".
	Region string
	// Percent is the rate in basis points of the table's basis.
	Percent int
}

// Validate checks the rate against the table's basis.
func (r Rate) Validate(basis int) error {
	if basis <= 0 {
		return fmt.Errorf("tax: the basis is %d", basis)
	}
	if strings.TrimSpace(r.Region) == "" || len(r.Region) > 8 {
		return fmt.Errorf("tax: %q is not a region code", r.Region)
	}
	if r.Percent < 0 || r.Percent > basis {
		return fmt.Errorf("tax: rate %q is %d of a basis of %d", r.Region, r.Percent, basis)
	}
	return nil
}

// Of returns the tax on a base. The part is computed with the shared minor-unit split, so a tax line
// and the invoice it belongs to round the same way and a half minor unit goes up.
func (r Rate) Of(basis int, base money.Amount) (money.Amount, error) {
	if err := r.Validate(basis); err != nil {
		return money.Amount{}, err
	}
	if err := base.Validate(); err != nil {
		return money.Amount{}, err
	}
	if _, err := base.NonNegative(); err != nil {
		return money.Amount{}, err
	}
	minor, err := money.Prorate(base.Minor, int64(r.Percent), int64(basis))
	if err != nil {
		return money.Amount{}, err
	}
	return money.New(minor, base.Currency)
}

// String renders the rate the way an invoice note and the console carry it.
func (r Rate) String() string {
	return fmt.Sprintf("%s %.2f%%", r.Region, float64(r.Percent)/100)
}

// Table is the set of rates a deployment applies, with the basis every rate is expressed in.
type Table struct {
	basis int
	rates map[string]Rate
}

// NewTable builds the rate table. A region declared twice is refused: two rates for one region are
// two answers to the same question, and the invoice can only carry one.
func NewTable(basis int, rates ...Rate) (*Table, error) {
	if basis <= 0 {
		return nil, fmt.Errorf("tax: the basis is %d", basis)
	}
	table := &Table{basis: basis, rates: make(map[string]Rate, len(rates))}
	for _, rate := range rates {
		if err := rate.Validate(basis); err != nil {
			return nil, err
		}
		region := strings.ToUpper(rate.Region)
		if _, exists := table.rates[region]; exists {
			return nil, fmt.Errorf("tax: region %q is declared twice", region)
		}
		rate.Region = region
		table.rates[region] = rate
	}
	return table, nil
}

// Basis returns the denominator the rates are expressed in.
func (t *Table) Basis() int { return t.basis }

// Rate returns the rate of one region. A region the table does not carry is an error: a deployment
// that silently taxed an unknown region at zero would under-collect and report a clean invoice.
func (t *Table) Rate(region string) (Rate, error) {
	rate, ok := t.rates[strings.ToUpper(strings.TrimSpace(region))]
	if !ok {
		return Rate{}, fmt.Errorf("tax: no rate for region %q", region)
	}
	return rate, nil
}

// Apply returns the tax one region charges on a base.
func (t *Table) Apply(region string, base money.Amount) (money.Amount, error) {
	rate, err := t.Rate(region)
	if err != nil {
		return money.Amount{}, err
	}
	return rate.Of(t.basis, base)
}

// Regions lists the declared regions in order.
func (t *Table) Regions() []string {
	regions := make([]string, 0, len(t.rates))
	for region := range t.rates {
		regions = append(regions, region)
	}
	sort.Strings(regions)
	return regions
}

// DefaultTable is the rate table a demonstration deployment starts with.
func DefaultTable(basis int) (*Table, error) {
	return NewTable(basis,
		Rate{Region: "ES", Percent: 2100},
		Rate{Region: "DE", Percent: 1900},
		Rate{Region: "FR", Percent: 2000},
		Rate{Region: "GB", Percent: 2000},
		Rate{Region: "US", Percent: 0},
	)
}
