package quota

import (
	"errors"
	"sharedquota/store"
	"time"
)

// Limiter represents one gateway. Gateways may share a Store.
type Limiter struct {
	config Config
	store  *store.Store
	stats  *metrics
}

type Slot struct {
	key      string
	id       uint64
	deadline time.Time
}

func New(config Config, backend *store.Store) (*Limiter, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if backend == nil {
		return nil, errors.New("shared store is required")
	}
	return &Limiter{config: config, store: backend, stats: &metrics{}}, nil
}

func (s Slot) Deadline() time.Time { return s.deadline }
func (s Slot) Valid() bool         { return s.id != 0 }

func (l *Limiter) Config() Config { return l.config }

// Usage reads shared occupancy without reserving capacity.
func (l *Limiter) Usage(tenant string) (int, error) {
	key, err := tenantKey(l.config.Namespace, tenant)
	if err != nil {
		return 0, err
	}
	var count int
	l.store.Transaction(key, func(tx *store.Tx) { count = tx.Count() })
	return count, nil
}
