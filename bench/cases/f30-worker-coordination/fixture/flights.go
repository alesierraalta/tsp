package workqueue

import "sync"

type FlightStats struct {
	Started uint64
	Joined  uint64
	Active  int
}

type FlightGroup struct {
	mu      sync.Mutex
	rows    map[string]*Future
	started uint64
	joined  uint64
}

func NewFlightGroup() *FlightGroup {
	return &FlightGroup{rows: make(map[string]*Future)}
}

func (g *FlightGroup) Do(key string, load func() (string, error)) *Future {
	g.mu.Lock()
	if f, ok := g.rows[key]; ok {
		g.joined++
		g.mu.Unlock()
		return f
	}
	f := newFuture()
	g.rows[key] = f
	g.started++
	g.mu.Unlock()
	go func() {
		value, err := load()
		g.mu.Lock()
		delete(g.rows, key)
		f.complete(Result{Value: value, Err: err})
		g.mu.Unlock()
	}()
	return f
}

func (g *FlightGroup) Stats() FlightStats {
	g.mu.Lock()
	defer g.mu.Unlock()
	return FlightStats{Started: g.started, Joined: g.joined, Active: len(g.rows)}
}
