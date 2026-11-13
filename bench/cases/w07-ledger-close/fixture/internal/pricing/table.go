// Package pricing holds the rate book: for each account's table, the basis points the ledger keeps
// on each metric, banded by the month's quantity.
package pricing

import (
	"errors"
	"fmt"
	"regexp"
	"sync"

	"example.com/ledgerclose/internal/usage"
)

// Errors the book reports.
var (
	ErrInvalid  = errors.New("pricing: invalid rate")
	ErrUnknown  = errors.New("pricing: unknown table")
	ErrNoMetric = errors.New("pricing: no rate for the metric")
)

// tableRE is the rate table id rule: a path-free lowercase id, like an account id.
var tableRE = regexp.MustCompile(`^[a-z][a-z0-9-]{1,30}[a-z0-9]$`)

// MaxTiers bounds the bands one rate may carry: a charge is a bounded walk and a table a person can read.
const MaxTiers = 8

// Tier is one quantity band. UpTo is the cumulative quantity the band ends at; the last band of a
// rate runs to the end of the account's quantity and its UpTo is ignored by the walk but still
// ordered.
type Tier struct {
	UpTo        int64 `json:"up_to"`
	BasisPoints int64 `json:"basis_points"`
}

// Rate is the rate of one metric in one table.
type Rate struct {
	Metric string `json:"metric"`
	Tiers  []Tier `json:"tiers"`
}

// Validate refuses a rate the ledger cannot apply: an unknown metric, no tiers, more tiers than the
// cap, bands that do not strictly increase, or a rate outside 0..10000 basis points.
func (r Rate) Validate() error {
	if !usage.ValidMetric(r.Metric) {
		return fmt.Errorf("%w: metric %q", ErrInvalid, r.Metric)
	}
	if len(r.Tiers) == 0 || len(r.Tiers) > MaxTiers {
		return fmt.Errorf("%w: %d tiers", ErrInvalid, len(r.Tiers))
	}
	var previous int64
	for i, tier := range r.Tiers {
		if tier.BasisPoints < 0 || tier.BasisPoints > 10_000 {
			return fmt.Errorf("%w: %s tier %d basis points %d", ErrInvalid, r.Metric, i, tier.BasisPoints)
		}
		if i == len(r.Tiers)-1 {
			break
		}
		if tier.UpTo <= previous {
			return fmt.Errorf("%w: %s tier %d does not increase", ErrInvalid, r.Metric, i)
		}
		previous = tier.UpTo
	}
	return nil
}

// Book is the set of rate tables one deployment prices with, keyed by table id and metric.
type Book struct {
	mu     sync.RWMutex
	tables map[string]map[string]Rate
}

// NewBook returns an empty book.
func NewBook() *Book {
	return &Book{tables: make(map[string]map[string]Rate)}
}

// Put registers a rate. The table id and the rate are validated here and nowhere else, so Band can
// trust that every rate it reads carries a priced metric and basis points inside the range.
func (b *Book) Put(tableID string, rate Rate) error {
	if !tableRE.MatchString(tableID) {
		return fmt.Errorf("%w: table %q", ErrInvalid, tableID)
	}
	if err := rate.Validate(); err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	metrics, ok := b.tables[tableID]
	if !ok {
		metrics = make(map[string]Rate, len(usage.Metrics))
		b.tables[tableID] = metrics
	}
	metrics[rate.Metric] = rate
	return nil
}

// Rate returns the rate of one metric in one table.
func (b *Book) Rate(tableID, metric string) (Rate, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	metrics, ok := b.tables[tableID]
	if !ok {
		return Rate{}, false
	}
	rate, ok := metrics[metric]
	return rate, ok
}

// HasTable reports whether the book carries a table at all.
func (b *Book) HasTable(tableID string) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	_, ok := b.tables[tableID]
	return ok
}

// Len reports how many rates the book carries across every table.
func (b *Book) Len() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	total := 0
	for _, metrics := range b.tables {
		total += len(metrics)
	}
	return total
}

// Band returns the tier index and the basis points a metric's month falls in. The rate is read from
// the book by table id and metric, never built from a caller's string: an account's table id is
// validated at registration and a metric is one of the enumerated ones. The walk over the bands is
// bounded by MaxTiers.
func (b *Book) Band(tableID, metric string, quantity int64) (int, int64, error) {
	rate, ok := b.Rate(tableID, metric)
	if !ok {
		if !b.HasTable(tableID) {
			return 0, 0, fmt.Errorf("%w: %q", ErrUnknown, tableID)
		}
		return 0, 0, fmt.Errorf("%w: %s in %q", ErrNoMetric, metric, tableID)
	}
	if quantity <= 0 {
		return 0, 0, fmt.Errorf("%w: quantity %d", ErrInvalid, quantity)
	}
	index, ratio := rate.tierFor(quantity)
	return index, ratio, nil
}
