package audit

import (
	"example.com/archive/internal/clock"
	"fmt"
	"strings"
	"time"
)

// Tail returns the last n entries, oldest first. Asking for more than the trail holds returns all of
// them.
func (t *Trail) Tail(n int) []Entry {
	entries := t.Entries()
	if n <= 0 || n >= len(entries) {
		return entries
	}
	return entries[len(entries)-n:]
}

// ForTenant returns the entries of one tenant, oldest first. The comparison is exact.
func (t *Trail) ForTenant(tenant string) []Entry {
	var out []Entry
	for _, entry := range t.Entries() {
		if entry.Tenant == tenant {
			out = append(out, entry)
		}
	}
	return out
}

// Since returns the entries recorded at or after an instant, oldest first.
func (t *Trail) Since(at time.Time) []Entry {
	stamp := clock.Stamp(at)
	var out []Entry
	for _, entry := range t.Entries() {
		if entry.At >= stamp {
			out = append(out, entry)
		}
	}
	return out
}

// Summarize renders the trail's shape for the operator page: how many entries, per outcome, and how
// many were dropped.
func (t *Trail) Summarize() string {
	counts := map[string]int{}
	entries := t.Entries()
	for _, entry := range entries {
		counts[entry.Outcome]++
	}
	parts := []string{
		fmt.Sprintf("%d entries", len(entries)),
		fmt.Sprintf("%d ok", counts[OutcomeOK]),
		fmt.Sprintf("%d refused", counts[OutcomeRefused]),
		fmt.Sprintf("%d failed", counts[OutcomeFailed]),
	}
	if dropped := t.Dropped(); dropped > 0 {
		parts = append(parts, fmt.Sprintf("%d dropped", dropped))
	}
	return strings.Join(parts, ", ")
}
