package tariff

import (
	"fmt"
	"sort"

	"example.com/shiptrack/internal/money"
)

// Band is already declared in book.go; this file holds the rules the book applies to a set of quotes,
// which is what a statement close needs: the surcharge a partner sees, the tax applied to it, and the
// allocation of both across the lines.

// Summary is the priced total of a set of quotes: the freight, the fuel and the sum.
type Summary struct {
	Lines int
	Base  money.Amount
	Fuel  money.Amount
	Net   money.Amount
}

// Summarise totals a set of quotes with checked arithmetic. A negative amount is refused: a credit
// belongs on its own statement line, not inside a total that a partner reads as charges.
func Summarise(quotes []Quote) (Summary, error) {
	var summary Summary
	for i, quote := range quotes {
		if quote.Base.Negative() || quote.Fuel.Negative() || quote.Total.Negative() {
			return Summary{}, fmt.Errorf("%w: quote %d is negative", ErrInvalid, i)
		}
		base, err := money.Add(summary.Base, quote.Base)
		if err != nil {
			return Summary{}, fmt.Errorf("%w: %v", ErrNoCharge, err)
		}
		fuel, err := money.Add(summary.Fuel, quote.Fuel)
		if err != nil {
			return Summary{}, fmt.Errorf("%w: %v", ErrNoCharge, err)
		}
		net, err := money.Add(summary.Net, quote.Total)
		if err != nil {
			return Summary{}, fmt.Errorf("%w: %v", ErrNoCharge, err)
		}
		summary = Summary{Lines: summary.Lines + 1, Base: base, Fuel: fuel, Net: net}
	}
	return summary, nil
}

// Tax applies the book's tax rate to a net amount, half to even. The rate is the book's, not a
// request's: a caller cannot ask for a statement to be taxed differently.
func (b *Book) Tax(net money.Amount) (money.Amount, error) {
	if net.Negative() {
		return 0, fmt.Errorf("%w: tax on a negative net %s", ErrInvalid, net)
	}
	tax, err := money.ApplyBasisPoints(net, b.taxBasisPoints)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrNoCharge, err)
	}
	return tax, nil
}

// Rates lists a service's bands as text, for the rate page. The ceilings are rendered in grams and the
// rates as amounts, so an operator reads the book the service applies.
func (s Service) Rates() []string {
	lines := make([]string, 0, len(s.Bands))
	for _, band := range s.Bands {
		lines = append(lines, fmt.Sprintf("up to %d g: %s/kg + %d bp", band.MaxGrams, band.PerKg, band.SurchargeBasisPoints))
	}
	if s.MinimumCharge.Positive() {
		lines = append(lines, fmt.Sprintf("minimum charge %s", s.MinimumCharge))
	}
	return lines
}

// SortedBands returns a copy of the bands in ceiling order.
func (s Service) SortedBands() []Band {
	bands := append([]Band(nil), s.Bands...)
	sort.Slice(bands, func(i, j int) bool { return bands[i].MaxGrams < bands[j].MaxGrams })
	return bands
}

// DefaultServices is the rate book a fresh deployment starts with: three service levels with whole-kilogram
// bands, a minimum charge and a fuel surcharge. A deployment that bills differently registers its own
// services; the defaults exist so a workspace is usable the moment it is opened, and every band's ceiling,
// rate and surcharge is inside the ranges AddService accepts.
func DefaultServices() []Service {
	return []Service{
		{
			Name:            "ground",
			MinimumCharge:   399,
			FuelBasisPoints: 850,
			Bands: []Band{
				{MaxGrams: 2000, PerKg: 349, SurchargeBasisPoints: 850},
				{MaxGrams: 10000, PerKg: 289, SurchargeBasisPoints: 850},
				{MaxGrams: 500000, PerKg: 219, SurchargeBasisPoints: 850},
			},
		},
		{
			Name:            "express",
			MinimumCharge:   899,
			FuelBasisPoints: 1100,
			Bands: []Band{
				{MaxGrams: 2000, PerKg: 749, SurchargeBasisPoints: 1100},
				{MaxGrams: 10000, PerKg: 639, SurchargeBasisPoints: 1100},
				{MaxGrams: 250000, PerKg: 519, SurchargeBasisPoints: 1100},
			},
		},
		{
			Name:            "freight",
			MinimumCharge:   4999,
			FuelBasisPoints: 620,
			Bands: []Band{
				{MaxGrams: 300000, PerKg: 149, SurchargeBasisPoints: 620},
				{MaxGrams: 1000000, PerKg: 119, SurchargeBasisPoints: 620},
			},
		},
	}
}
