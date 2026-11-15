package store

import (
	"strings"
)

// Filter is the query a listing applies. Every field is optional; an empty field matches everything.
// The filters are applied in memory because the index is in memory, and the comparison is written once
// so a handler cannot invent its own.
type Filter struct {
	// Tenant restricts the listing to one tenant. An empty tenant lists every tenant.
	Tenant string
	// Kind restricts the listing to one kind.
	Kind string
	// Batch restricts the listing to the objects of one batch.
	Batch string
	// Query is a case-insensitive substring of the name, the title or one of the tags.
	Query string
	// MinPages and MaxPages bound the declared page count; a zero bound is open.
	MinPages int
	MaxPages int
	// Limit caps the number of rows returned. A zero limit means the caller's own default.
	Limit int
	// Offset skips that many rows of the ordered result.
	Offset int
}

// Match reports whether one record passes the filter. The tenant comparison is exact; the query is
// the only comparison that folds case, because it matches text and not identity.
func (f Filter) Match(record Record) bool {
	if f.Tenant != "" && record.Tenant != f.Tenant {
		return false
	}
	if f.Kind != "" && record.Kind != f.Kind {
		return false
	}
	if f.Batch != "" && record.Batch != f.Batch {
		return false
	}
	if f.MinPages > 0 && record.Pages < f.MinPages {
		return false
	}
	if f.MaxPages > 0 && record.Pages > f.MaxPages {
		return false
	}
	if q := strings.ToLower(strings.TrimSpace(f.Query)); q != "" {
		if !f.queryMatch(record, q) {
			return false
		}
	}
	return true
}

// queryMatch is the substring rule the filter documents: name, title or any tag, lowercased.
func (f Filter) queryMatch(record Record, q string) bool {
	if strings.Contains(strings.ToLower(record.Name), q) {
		return true
	}
	if strings.Contains(strings.ToLower(record.Title), q) {
		return true
	}
	for _, tag := range record.Tags {
		if strings.Contains(strings.ToLower(tag), q) {
			return true
		}
	}
	return false
}

// List returns the records that pass f, newest first, with the offset and limit applied. A negative
// offset or limit is refused by the caller's own validation, not guessed here.
func (s *Store) List(f Filter) ([]Record, error) {
	records := s.All()
	matched := make([]Record, 0, len(records))
	for _, record := range records {
		if f.Match(record) {
			matched = append(matched, record)
		}
	}
	if f.Offset > 0 {
		if f.Offset >= len(matched) {
			return nil, nil
		}
		matched = matched[f.Offset:]
	}
	if f.Limit > 0 && len(matched) > f.Limit {
		matched = matched[:f.Limit]
	}
	return matched, nil
}

// CountMatching returns how many records pass f, ignoring offset and limit.
func (s *Store) CountMatching(f Filter) int {
	total := 0
	for _, record := range s.All() {
		if f.Match(record) {
			total++
		}
	}
	return total
}
