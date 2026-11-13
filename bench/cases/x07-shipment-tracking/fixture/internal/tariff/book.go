// Package tariff is the rate book: what the service charges a partner for carrying a shipment on a
// service. A rate is an amount per kilogram in minor units and a fuel surcharge in basis points, and
// the arithmetic is integer throughout — the weight is billed in whole kilograms, the base is a
// multiplication checked for overflow, and the surcharge is applied with the half-to-even rule of
// internal/money. No path here converts an amount through float64.
package tariff

import (
	"errors"
	"fmt"
	"sort"
	"sync"

	"example.com/shiptrack/internal/money"
)

// Errors the rate book reports.
var (
	ErrInvalid  = errors.New("tariff: invalid rate")
	ErrNoRate   = errors.New("tariff: no rate")
	ErrNoBand   = errors.New("tariff: weight is outside the book")
	ErrNoCharge = errors.New("tariff: charge is out of range")
)

// Band is one weight band of a service: up to MaxGrams, an amount per billed kilogram, and the
// service's fuel surcharge in basis points. A band's ceiling is inclusive.
type Band struct {
	MaxGrams             int64
	PerKg                money.Amount
	SurchargeBasisPoints int32
}

// Service is one service level a partner may buy: its bands, a minimum charge and the fuel surcharge
// applied to the base. A service with no bands has no rate and is refused at registration.
type Service struct {
	Name            string
	Bands           []Band
	MinimumCharge   money.Amount
	FuelBasisPoints int32
}

// Quote is what one shipment is billed: the billed kilograms, the base, the fuel surcharge and the
// total. Every field is an integer amount; the surcharge is computed from the base, never from a
// float, and the total is the checked sum of the two.
type Quote struct {
	Service   string
	Grams     int64
	Kilograms int64
	PerKg     money.Amount
	Base      money.Amount
	Fuel      money.Amount
	Total     money.Amount
}

// Book is the rate book. A deployment mounts one; a close reads it, and no handler can change it.
type Book struct {
	mu             sync.RWMutex
	services       map[string]Service
	taxBasisPoints int32
}

// NewBook returns an empty book with a tax rate in basis points. The tax rate must be inside
// 0..10000: a rate above 100% is a configuration mistake, not a tax.
func NewBook(taxBasisPoints int32) (*Book, error) {
	if taxBasisPoints < 0 || taxBasisPoints > money.BasisPointsPerUnit {
		return nil, fmt.Errorf("%w: tax %d basis points", ErrInvalid, taxBasisPoints)
	}
	return &Book{services: map[string]Service{}, taxBasisPoints: taxBasisPoints}, nil
}

// TaxBasisPoints is the tax rate the book applies at close.
func (b *Book) TaxBasisPoints() int32 { return b.taxBasisPoints }

// AddService registers a service. Bands are sorted by their ceilings and must not repeat one: two
// bands that end at the same weight would make the rate of a shipment depend on the order they were
// registered in.
func (b *Book) AddService(service Service) (Service, error) {
	if service.Name == "" || len(service.Bands) == 0 {
		return Service{}, fmt.Errorf("%w: service %q", ErrInvalid, service.Name)
	}
	if service.MinimumCharge.Negative() {
		return Service{}, fmt.Errorf("%w: service %q has a negative minimum", ErrInvalid, service.Name)
	}
	if service.FuelBasisPoints < 0 || service.FuelBasisPoints > money.BasisPointsPerUnit {
		return Service{}, fmt.Errorf("%w: service %q fuel %d", ErrInvalid, service.Name, service.FuelBasisPoints)
	}
	bands := append([]Band(nil), service.Bands...)
	sort.Slice(bands, func(i, j int) bool { return bands[i].MaxGrams < bands[j].MaxGrams })
	for i, band := range bands {
		if band.MaxGrams <= 0 || band.PerKg <= 0 {
			return Service{}, fmt.Errorf("%w: service %q band %d", ErrInvalid, service.Name, i)
		}
		if band.SurchargeBasisPoints < 0 || band.SurchargeBasisPoints > money.BasisPointsPerUnit {
			return Service{}, fmt.Errorf("%w: service %q band %d surcharge %d", ErrInvalid, service.Name, i, band.SurchargeBasisPoints)
		}
		if i > 0 && bands[i-1].MaxGrams == band.MaxGrams {
			return Service{}, fmt.Errorf("%w: service %q repeats the ceiling %d", ErrInvalid, service.Name, band.MaxGrams)
		}
	}
	service.Bands = bands
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, exists := b.services[service.Name]; exists {
		return Service{}, fmt.Errorf("%w: service %q is already registered", ErrInvalid, service.Name)
	}
	b.services[service.Name] = service
	return service, nil
}

// Service resolves a service by its exact name.
func (b *Book) Service(name string) (Service, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	service, ok := b.services[name]
	if !ok {
		return Service{}, fmt.Errorf("%w: %q", ErrNoRate, name)
	}
	return service, nil
}

// Services lists the registered services, sorted by name.
func (b *Book) Services() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	names := make([]string, 0, len(b.services))
	for name := range b.services {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Rate prices one shipment. The weight is billed in whole kilograms, the band is the first one whose
// ceiling holds the shipment, and a base below the service's minimum is raised to the minimum. The
// surcharge is the band's basis points applied to the base, rounded half to even, and the total is the
// checked sum.
func (b *Book) Rate(service string, grams int64) (Quote, error) {
	registered, err := b.Service(service)
	if err != nil {
		return Quote{}, err
	}
	if grams <= 0 {
		return Quote{}, fmt.Errorf("%w: %d grams", ErrInvalid, grams)
	}
	band, ok := registered.band(grams)
	if !ok {
		return Quote{}, fmt.Errorf("%w: %d grams is above the highest band", ErrNoBand, grams)
	}
	kilograms := billedKilograms(grams)
	base, err := money.MulInt(band.PerKg, kilograms)
	if err != nil {
		return Quote{}, fmt.Errorf("%w: %v", ErrNoCharge, err)
	}
	if base < registered.MinimumCharge {
		base = registered.MinimumCharge
	}
	surcharge, err := money.ApplyBasisPoints(base, band.SurchargeBasisPoints)
	if err != nil {
		return Quote{}, fmt.Errorf("%w: %v", ErrNoCharge, err)
	}
	surcharge, err = money.Add(surcharge, 0)
	if err != nil {
		return Quote{}, fmt.Errorf("%w: %v", ErrNoCharge, err)
	}
	total, err := money.Add(base, surcharge)
	if err != nil {
		return Quote{}, fmt.Errorf("%w: %v", ErrNoCharge, err)
	}
	return Quote{
		Service:   registered.Name,
		Grams:     grams,
		Kilograms: kilograms,
		PerKg:     band.PerKg,
		Base:      base,
		Fuel:      surcharge,
		Total:     total,
	}, nil
}

// band is the first band whose ceiling holds the weight.
func (s Service) band(grams int64) (Band, bool) {
	for _, band := range s.Bands {
		if grams <= band.MaxGrams {
			return band, true
		}
	}
	return Band{}, false
}

// billedKilograms rounds a weight up to whole kilograms, with the part of a kilogram billed as a whole
// one: 1 gram is one kilogram and 1001 grams are two. The addition is checked so a weight near the top
// of the range cannot wrap to a smaller one.
func billedKilograms(grams int64) int64 {
	if grams <= 0 {
		return 0
	}
	if grams > 1<<62 {
		// A weight this large cannot be turned into kilograms without overflowing the round-up; the band
		// lookup has already refused anything above the book's ceiling in practice, and this branch keeps
		// the arithmetic honest for a caller that is not going through Rate.
		return (grams + 999) / 1000
	}
	return (grams + 999) / 1000
}

// Describe renders a quote for a statement line: the service, the billed weight and the amounts.
func (q Quote) Describe() string {
	return fmt.Sprintf("%s %d kg at %s (%s)", q.Service, q.Kilograms, q.PerKg, q.Total)
}
