package quota

import "sync"

// Snapshot is local to a gateway and does not represent shared occupancy.
type Snapshot struct {
	Accepted uint64
	Rejected uint64
	Released uint64
}

type metrics struct {
	mu    sync.Mutex
	value Snapshot
}

func (m *metrics) accepted() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.value.Accepted++
}

func (m *metrics) rejected() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.value.Rejected++
}

func (m *metrics) released() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.value.Released++
}

func (l *Limiter) Snapshot() Snapshot {
	l.stats.mu.Lock()
	defer l.stats.mu.Unlock()
	return l.stats.value
}
