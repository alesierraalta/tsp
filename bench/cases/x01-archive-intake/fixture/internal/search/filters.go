package search

import (
	"fmt"
	"strconv"
	"strings"

	"example.com/archive/internal/store"
)

// Filter is the query string form the HTTP surface accepts: q, kind, tenant, pages_min, pages_max and
// limit, each optional. Parsing is done once, here, so a handler cannot read a filter field by itself
// and forget a bound.
type Query struct {
	Text     string
	Kind     string
	Tenant   string
	PagesMin int
	PagesMax int
	Limit    int
	Batch    string
}

// ParseQuery reads the query-string form. An unknown key is refused rather than ignored: a filter the
// service does not implement would otherwise answer with an unfiltered listing.
func ParseQuery(values map[string]string, defaultLimit, maxLimit int) (Query, error) {
	q := Query{Limit: defaultLimit}
	known := map[string]bool{"q": true, "kind": true, "tenant": true, "pages_min": true, "pages_max": true, "limit": true, "batch": true}
	for key := range values {
		if !known[key] {
			return Query{}, fmt.Errorf("search: unknown filter %q", key)
		}
	}
	q.Text = strings.TrimSpace(values["q"])
	q.Kind = strings.TrimSpace(values["kind"])
	q.Tenant = strings.TrimSpace(values["tenant"])
	q.Batch = strings.TrimSpace(values["batch"])
	for key, target := range map[string]*int{"pages_min": &q.PagesMin, "pages_max": &q.PagesMax} {
		raw := strings.TrimSpace(values[key])
		if raw == "" {
			continue
		}
		n, err := strconv.Atoi(raw)
		if err != nil {
			return Query{}, fmt.Errorf("search: %s=%q is not a number", key, raw)
		}
		if n < 0 {
			return Query{}, fmt.Errorf("search: %s=%q is negative", key, raw)
		}
		*target = n
	}
	if raw := strings.TrimSpace(values["limit"]); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			return Query{}, fmt.Errorf("search: limit=%q is not a number", raw)
		}
		if n < 1 {
			return Query{}, fmt.Errorf("search: limit=%q is not positive", raw)
		}
		if maxLimit > 0 && n > maxLimit {
			return Query{}, fmt.Errorf("search: limit=%d is more than the %d this endpoint serves", n, maxLimit)
		}
		q.Limit = n
	}
	if q.PagesMax > 0 && q.PagesMin > q.PagesMax {
		return Query{}, fmt.Errorf("search: pages_min %d is above pages_max %d", q.PagesMin, q.PagesMax)
	}
	return q, nil
}

// StoreFilter is the store filter the query string form means. The kinds are mapped to the store's
// exact kind values; an unknown kind is passed through and matches nothing, which is better than
// answering with everything.
func (q Query) StoreFilter() store.Filter {
	return store.Filter{
		Tenant:   q.Tenant,
		Kind:     q.Kind,
		Batch:    q.Batch,
		Query:    q.Text,
		MinPages: q.PagesMin,
		MaxPages: q.PagesMax,
		Limit:    q.Limit,
	}
}

// String renders the query back as a query string, for the pagination links a page prints.
func (q Query) String() string {
	var parts []string
	add := func(key, value string) {
		if value == "" {
			return
		}
		parts = append(parts, key+"="+value)
	}
	add("q", q.Text)
	add("kind", q.Kind)
	add("tenant", q.Tenant)
	add("batch", q.Batch)
	if q.PagesMin > 0 {
		add("pages_min", strconv.Itoa(q.PagesMin))
	}
	if q.PagesMax > 0 {
		add("pages_max", strconv.Itoa(q.PagesMax))
	}
	if q.Limit > 0 {
		add("limit", strconv.Itoa(q.Limit))
	}
	return strings.Join(parts, "&")
}
