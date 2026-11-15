// Package reports defines the reports the billing console runs. A report is declared once — a table,
// a column list, the filters it accepts, an ordering and a row bound — and every route that serves it
// runs that declaration through internal/query, which binds the values as arguments of the statement.
package reports

import (
	"fmt"
	"sort"
	"strings"

	"example.com/billing/internal/query"
	"example.com/billing/internal/sqlrun"
	"example.com/billing/internal/store"
)

// Definition is one report.
type Definition struct {
	// Name is the segment the report is served under.
	Name string
	// Title is what the console prints above the table.
	Title string
	// Table is the projection table the report reads.
	Table string
	// Columns are the columns the report selects, in the order it prints them.
	Columns []string
	// Filters are the query parameters the report accepts, each naming the column it narrows.
	Filters []query.Filter
	// Order is the column the rows are ordered by.
	Order string
	// Desc orders the rows descending when it is set.
	Desc bool
	// Limit is the largest number of rows the report returns.
	Limit int
}

// Validate checks the definition. A definition without a bound is refused here, at start-up, rather
// than read a whole table on the first request.
func (d Definition) Validate() error {
	if !query.ValidName(d.Name) {
		return fmt.Errorf("reports: %q is not a report name", d.Name)
	}
	if strings.TrimSpace(d.Title) == "" {
		return fmt.Errorf("reports: report %q has no title", d.Name)
	}
	if !query.ValidName(d.Table) {
		return fmt.Errorf("reports: report %q reads table %q", d.Name, d.Table)
	}
	if len(d.Columns) == 0 {
		return fmt.Errorf("reports: report %q selects no columns", d.Name)
	}
	for _, column := range d.Columns {
		if !query.ValidName(column) {
			return fmt.Errorf("reports: report %q selects column %q", d.Name, column)
		}
	}
	if !query.ValidName(d.Order) {
		return fmt.Errorf("reports: report %q orders by %q", d.Name, d.Order)
	}
	if d.Limit <= 0 {
		return fmt.Errorf("reports: report %q carries no row bound", d.Name)
	}
	seen := make(map[string]bool, len(d.Filters))
	for _, filter := range d.Filters {
		if err := filter.Validate(); err != nil {
			return fmt.Errorf("reports: report %q: %w", d.Name, err)
		}
		if seen[filter.Param] {
			return fmt.Errorf("reports: report %q declares filter %q twice", d.Name, filter.Param)
		}
		seen[filter.Param] = true
	}
	return nil
}

// Builder returns the statement's opening: the declared columns over the declared table, narrowed to
// the caller's tenant. The tenant is the first condition of every report and no filter replaces it,
// which is what keeps a report from widening to another tenant's rows.
func (d Definition) Builder(tenant string) *query.Builder {
	return query.Select(d.Columns...).From(d.Table).Where("tenant", tenant)
}

// Stmt renders the statement a request runs. The filters a caller sends are read through the
// declaration, so a parameter the report does not declare is refused instead of ignored, and every
// value is bound as an argument.
func (d Definition) Stmt(tenant string, filters map[string]string, limit int) (query.Stmt, error) {
	if tenant == "" {
		return query.Stmt{}, fmt.Errorf("reports: report %q needs a tenant", d.Name)
	}
	bound := limit
	if bound <= 0 || bound > d.Limit {
		bound = d.Limit
	}
	b := d.Builder(tenant)
	if err := query.Read(b, d.Filters, filters); err != nil {
		return query.Stmt{}, err
	}
	if err := b.Limit(bound); err != nil {
		return query.Stmt{}, err
	}
	b.OrderBy(d.Order, d.Desc)
	return b.Stmt()
}

// Answer is the result of a report: its columns, its rows as text and the tenant it answered for.
type Answer struct {
	Report  string     `json:"report"`
	Title   string     `json:"title"`
	Columns []string   `json:"columns"`
	Rows    [][]string `json:"rows"`
	Tenant  string     `json:"tenant"`
	Limit   int        `json:"limit"`
	Filters []string   `json:"filters"`
}

// Len returns the number of rows answered.
func (a Answer) Len() int { return len(a.Rows) }

// Column returns the index of a column of the answer, or -1.
func (a Answer) Column(name string) int {
	for i, column := range a.Columns {
		if column == name {
			return i
		}
	}
	return -1
}

// Registry is the set of reports a deployment serves.
type Registry struct {
	byName map[string]Definition
	order  []string
}

// NewRegistry validates the definitions and returns them indexed by name. Two reports may not share a
// name, and a definition that fails validation stops the start-up: a report that cannot be run is not
// served at all.
func NewRegistry(definitions ...Definition) (*Registry, error) {
	r := &Registry{byName: make(map[string]Definition, len(definitions))}
	for _, definition := range definitions {
		if err := definition.Validate(); err != nil {
			return nil, err
		}
		if _, exists := r.byName[definition.Name]; exists {
			return nil, fmt.Errorf("reports: %q is declared twice", definition.Name)
		}
		r.byName[definition.Name] = definition
		r.order = append(r.order, definition.Name)
	}
	sort.Strings(r.order)
	return r, nil
}

// Get returns one report definition.
func (r *Registry) Get(name string) (Definition, error) {
	definition, ok := r.byName[name]
	if !ok {
		return Definition{}, fmt.Errorf("%w: %q", ErrNoReport, name)
	}
	return definition, nil
}

// Names lists the report names in order.
func (r *Registry) Names() []string {
	return append([]string(nil), r.order...)
}

// Summary is the catalog entry `GET /api/reports` returns: what a report answers and what it accepts.
type Summary struct {
	Name    string       `json:"name"`
	Title   string       `json:"title"`
	Columns []string     `json:"columns"`
	Filters []FilterView `json:"filters"`
	Limit   int          `json:"limit"`
}

// FilterView is one accepted filter as the catalog publishes it.
type FilterView struct {
	Param    string   `json:"param"`
	Column   string   `json:"column"`
	Accepted []string `json:"accepted,omitempty"`
}

// Catalog lists every report a deployment serves.
func (r *Registry) Catalog() []Summary {
	out := make([]Summary, 0, len(r.order))
	for _, name := range r.order {
		definition := r.byName[name]
		views := make([]FilterView, 0, len(definition.Filters))
		for _, filter := range definition.Filters {
			views = append(views, FilterView{
				Param: filter.Param, Column: filter.Column, Accepted: filter.Values,
			})
		}
		out = append(out, Summary{
			Name: definition.Name, Title: definition.Title,
			Columns: definition.Columns, Filters: views, Limit: definition.Limit,
		})
	}
	return out
}

// Run answers one report over the store's projection.
func (r *Registry) Run(s *store.Store, name, tenant string, filters map[string]string, limit int) (Answer, error) {
	definition, err := r.Get(name)
	if err != nil {
		return Answer{}, err
	}
	stmt, err := definition.Stmt(tenant, filters, limit)
	if err != nil {
		return Answer{}, err
	}
	answer := Answer{
		Report: definition.Name, Title: definition.Title, Columns: definition.Columns,
		Tenant: tenant, Limit: min(limit, definition.Limit), Filters: query.SortedKeys(filters),
	}
	result, err := s.Query(stmt)
	if err != nil {
		return Answer{}, fmt.Errorf("reports: %s: %w", definition.Name, err)
	}
	answer.Rows = rowsOf(result)
	if answer.Limit <= 0 {
		answer.Limit = definition.Limit
	}
	return answer, nil
}

// rowsOf renders the engine's answer as text rows.
func rowsOf(result sqlrun.Result) [][]string {
	return result.Strings()
}

// min returns the smaller of two counts.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
