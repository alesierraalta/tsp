package backfill

import "sync"

type Registry struct {
	mu     sync.Mutex
	limit  int
	active map[uint64]bool
	next   uint64
}

type Connection struct {
	registry *Registry
}

func NewRegistry(limit int) *Registry {
	if limit < 1 {
		limit = 1
	}
	return &Registry{limit: limit, active: make(map[uint64]bool)}
}

func (r *Registry) Open() *Connection {
	return &Connection{registry: r}
}

func (c *Connection) Acquire() (uint64, bool) {
	r := c.registry
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.active) >= r.limit {
		return 0, false
	}
	r.next++
	r.active[r.next] = true
	return r.next, true
}

func (c *Connection) Release(ticket uint64) {
	r := c.registry
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.active, ticket)
}

func (r *Registry) Active() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.active)
}
