// Package reports defines the reports the service serves and runs them.
package reports

import (
	"fmt"

	"example.com/insight/internal/query"
	"example.com/insight/internal/store"
)

// Definition is one report: the table it reads, the columns it returns, the filters a caller may
// pass, and the ordering and bound it applies. A caller never chooses a table or a column.
type Definition struct {
	Name    string
	Title   string
	Table   string
	Columns []string
	Filters []query.Filter
	OrderBy string
	Desc    bool
	Limit   int
}

// Registry is the report list of the service.
type Registry struct {
	defs []Definition
}

// NewRegistry returns the reports the service ships with.
func NewRegistry() *Registry {
	defs := []Definition{
		{
			Name:    "usage",
			Title:   "Metered usage",
			Table:   store.TableEvents,
			Columns: []string{"id", "tenant", "account", "status", "amount", "created"},
			Filters: []query.Filter{
				{Param: "status", Column: "status", Operator: "="},
				{Param: "account", Column: "account", Operator: "="},
			},
			OrderBy: "id",
			Limit:   200,
		},
		{
			Name:    "archive",
			Title:   "Archived events",
			Table:   store.TableEvents,
			Columns: []string{"id", "account", "amount", "created"},
			Filters: []query.Filter{
				{Param: "account", Column: "account", Operator: "="},
			},
			OrderBy: "created",
			Limit:   200,
		},
		{
			Name:    "quota",
			Title:   "Quota usage",
			Table:   store.TableQuotas,
			Columns: []string{"tenant", "account", "limit", "used"},
			Filters: []query.Filter{
				{Param: "account", Column: "account", Operator: "="},
			},
			OrderBy: "account",
			Limit:   200,
		},
	}
	return &Registry{defs: defs}
}

// Lookup returns one report by name.
func (r *Registry) Lookup(name string) (Definition, error) {
	for _, def := range r.defs {
		if def.Name == name {
			return def, nil
		}
	}
	return Definition{}, fmt.Errorf("reports: no report %q", name)
}

// Names lists the report names in declaration order.
func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.defs))
	for _, def := range r.defs {
		out = append(out, def.Name)
	}
	return out
}

// Definitions returns the definitions in declaration order, for the catalogue endpoint.
func (r *Registry) Definitions() []Definition { return append([]Definition(nil), r.defs...) }
