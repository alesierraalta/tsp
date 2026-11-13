package audit

import (
	"sort"
	"strings"
)

// Filter selects trail entries. Every field is optional; an empty field matches every entry. The
// comparisons are exact except the query, which is a case-insensitive substring of the detail.
type Filter struct {
	Tenant  string
	Subject string
	Action  string
	Outcome string
	Query   string
	Limit   int
}

// Match reports whether one entry passes the filter.
func (f Filter) Match(entry Entry) bool {
	if f.Tenant != "" && entry.Tenant != f.Tenant {
		return false
	}
	if f.Subject != "" && entry.Subject != f.Subject {
		return false
	}
	if f.Action != "" && entry.Action != f.Action {
		return false
	}
	if f.Outcome != "" && entry.Outcome != f.Outcome {
		return false
	}
	if q := strings.TrimSpace(f.Query); q != "" {
		haystack := strings.ToLower(entry.Target + " " + entry.Detail + " " + entry.Subject)
		if !strings.Contains(haystack, strings.ToLower(q)) {
			return false
		}
	}
	return true
}

// Select returns the entries that pass f, newest first, capped by f.Limit when it is positive.
func (t *Trail) Select(f Filter) []Entry {
	var out []Entry
	for _, entry := range t.Entries() {
		if f.Match(entry) {
			out = append(out, entry)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At > out[j].At })
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out
}

// Describe renders one entry as the line the audit page prints.
func (e Entry) Describe() string {
	parts := []string{e.At, e.Subject + "@" + e.Tenant, e.Action, e.Target, e.Outcome}
	if e.Detail != "" {
		parts = append(parts, e.Detail)
	}
	return strings.Join(parts, " | ")
}

// Actions returns the distinct actions the trail carries, sorted, for the audit page's filter select.
func (t *Trail) Actions() []string {
	seen := map[string]bool{}
	for _, entry := range t.Entries() {
		seen[entry.Action] = true
	}
	out := make([]string, 0, len(seen))
	for action := range seen {
		out = append(out, action)
	}
	sort.Strings(out)
	return out
}
