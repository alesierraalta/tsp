package workqueue

import (
	"errors"
	"sync"
	"time"
)

type Manager struct {
	mu        sync.Mutex
	book      *LeaseBook
	config    Config
	completed map[string]int
}

func NewManager(config Config) (*Manager, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	return &Manager{book: NewLeaseBook(), config: config, completed: make(map[string]int)}, nil
}

func (m *Manager) Start(key, owner string, now time.Time) (Lease, bool) {
	if !validKey(key) || !validKey(owner) {
		return Lease{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.book.Acquire(namespaced(m.config.Prefix, key), owner, now, m.config.LeaseTTL)
}

func (m *Manager) Finish(lease Lease) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	current, ok := m.book.Current(lease.Key)
	if !ok || current.Owner != lease.Owner {
		return errors.New("lease unavailable")
	}
	if !m.book.Release(lease.Key, current.Generation) {
		return errors.New("lease changed")
	}
	m.completed[lease.Key]++
	return nil
}

func (m *Manager) Active() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.book.Count()
}

func (m *Manager) Completions(key string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.completed[namespaced(m.config.Prefix, key)]
}
