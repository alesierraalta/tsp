package publisher

import "sort"

// Plan revisions are assigned by the planner before publication.
type Plan struct {
	Revision uint64
	Routes   []string
	State    string
}

// Store operations are atomic.
type Store interface {
	Stage(Plan)
	Publish(uint64)
	Head() (Plan, bool)
	Candidates(uint64, string) []uint64
	Delete(uint64)
}

type Publisher struct{ Store Store }

func (p Publisher) Prepare(plan Plan) func() {
	p.Store.Stage(plan)
	return func() { p.Store.Publish(plan.Revision) }
}

func (p Publisher) Current() []string {
	plan, ok := p.Store.Head()
	if !ok {
		return nil
	}
	return append([]string(nil), plan.Routes...)
}

// Sweep removes pending-deletion plans through the supplied revision.
func (p Publisher) Sweep(through uint64) int {
	ids := p.Store.Candidates(through, "pending-deletion")
	for _, id := range ids {
		p.Store.Delete(id)
	}
	return len(ids)
}

// MemoryStore is a single-step scheduler adapter; callers choose step order.
type MemoryStore struct {
	plans map[uint64]Plan
	head  uint64
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{plans: make(map[uint64]Plan)}
}

func (s *MemoryStore) Stage(plan Plan) {
	plan.Routes = append([]string(nil), plan.Routes...)
	s.plans[plan.Revision] = plan
}

func (s *MemoryStore) Publish(revision uint64) {
	s.head = revision
}

func (s *MemoryStore) Head() (Plan, bool) {
	plan, ok := s.plans[s.head]
	plan.Routes = append([]string(nil), plan.Routes...)
	return plan, ok
}

func (s *MemoryStore) Candidates(through uint64, state string) []uint64 {
	var ids []uint64
	for id, plan := range s.plans {
		if id <= through && plan.State == state {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func (s *MemoryStore) Delete(revision uint64) { delete(s.plans, revision) }

func (s *MemoryStore) Revisions() []uint64 {
	var ids []uint64
	for id := range s.plans {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}
