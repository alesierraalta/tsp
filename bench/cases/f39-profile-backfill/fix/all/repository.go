package backfill

import (
	"sort"
	"sync"
)

type Repository struct {
	mu   sync.Mutex
	rows map[string]Profile
}

func NewRepository(profiles []Profile) *Repository {
	r := &Repository{rows: make(map[string]Profile)}
	for _, p := range profiles {
		if p.State == "" {
			p.State = "pending"
		}
		r.rows[p.ID] = p
	}
	return r
}

func (r *Repository) Pending(o Options) []Profile {
	return r.selectRows(o, false)
}

func (r *Repository) Claim(o Options) []Profile {
	return r.selectRows(o, true)
}

func (r *Repository) selectRows(o Options, claim bool) []Profile {
	r.mu.Lock()
	defer r.mu.Unlock()
	ids := make([]string, 0)
	for id, p := range r.rows {
		if p.State == "pending" && (o.Team == "" || p.Team == o.Team) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	if len(ids) > o.Batch {
		ids = ids[:o.Batch]
	}
	result := make([]Profile, 0, len(ids))
	for _, id := range ids {
		p := r.rows[id]
		if claim {
			p.State = "claimed"
			p.Attempts++
			r.rows[id] = p
		}
		result = append(result, p)
	}
	return result
}

func (r *Repository) finish(id, state string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p := r.rows[id]
	p.State = state
	r.rows[id] = p
}

func (r *Repository) Snapshot() []Profile {
	r.mu.Lock()
	defer r.mu.Unlock()
	rows := make([]Profile, 0, len(r.rows))
	for _, p := range r.rows {
		rows = append(rows, p)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	return rows
}
