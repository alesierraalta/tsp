// Package report is the catalog of selections a partner or an operator may run. A report is a
// declaration: a table, the columns it shows, the filters it accepts, the orders it allows and the page
// it returns. A request fills the declaration in; it can never add a column, a filter or an order the
// declaration does not carry, and the SQL the report runs is rendered from the declaration with every
// value bound.
package report

import (
	"errors"
	"fmt"
	"net/url"

	"example.com/shiptrack/internal/binder"
)

// ErrUnknownReport is reported for a report name that is not in the catalog. A name is compared exactly
// and is never resolved to a nearest match: a report a request names is one the catalog declares.
var ErrUnknownReport = errors.New("report: unknown report")

// Definition is one report.
type Definition struct {
	Name        string
	Title       string
	Description string
	Declaration binder.Declaration
}

// definitions is the catalog. The report names are the names a request and an export path carry, so
// each one is also a name segment: a report name that would not resolve as a file name cannot be in
// the catalog.
var definitions = []Definition{
	{
		Name:        "events-by-carrier",
		Title:       "Scans by carrier",
		Description: "One row per carrier scan, newest first. Filter by carrier, kind, shipment or a half-open instant range.",
		Declaration: binder.Declaration{
			Table: "events",
			Columns: []string{
				"at", "shipment", "carrier", "kind", "facility", "grams",
			},
			Filters: []binder.Filter{
				{Name: "carrier", Column: "carrier", Kind: binder.KindText},
				{Name: "kind", Column: "kind", Kind: binder.KindText},
				{Name: "shipment", Column: "shipment", Kind: binder.KindText},
				{Name: "partner", Column: "partner", Kind: binder.KindText},
				{Name: "since", Column: "at", Kind: binder.KindTime, Op: binder.OpGe},
				{Name: "until", Column: "at", Kind: binder.KindTime, Op: binder.OpLt},
				{Name: "min-grams", Column: "grams", Kind: binder.KindInt, Op: binder.OpGe},
			},
			Orders: map[string]binder.Order{
				"newest":   {Column: "at", Desc: true},
				"oldest":   {Column: "at"},
				"heaviest": {Column: "grams", Desc: true},
			},
			DefaultOrder: []binder.Order{{Column: "at", Desc: true}},
			PageSize:     100,
			MaxRows:      500,
		},
	},
	{
		Name:        "shipments-by-lane",
		Title:       "Shipments by lane",
		Description: "One row per shipment, newest first. Filter by partner, lane, carrier, service or status.",
		Declaration: binder.Declaration{
			Table:   "shipments",
			Columns: []string{"created", "id", "partner", "carrier", "lane", "service", "grams", "status"},
			Filters: []binder.Filter{
				{Name: "partner", Column: "partner", Kind: binder.KindText},
				{Name: "lane", Column: "lane", Kind: binder.KindText},
				{Name: "carrier", Column: "carrier", Kind: binder.KindText},
				{Name: "service", Column: "service", Kind: binder.KindText},
				{Name: "status", Column: "status", Kind: binder.KindText},
				{Name: "since", Column: "created", Kind: binder.KindTime, Op: binder.OpGe},
			},
			Orders: map[string]binder.Order{
				"newest":   {Column: "created", Desc: true},
				"oldest":   {Column: "created"},
				"heaviest": {Column: "grams", Desc: true},
			},
			DefaultOrder: []binder.Order{{Column: "created", Desc: true}},
			PageSize:     100,
			MaxRows:      500,
		},
	},
	{
		Name:        "statements-by-partner",
		Title:       "Closed statements",
		Description: "One row per closed statement, newest period first. Filter by partner, tenant or period.",
		Declaration: binder.Declaration{
			Table:   "statements",
			Columns: []string{"period", "partner", "lines", "net", "tax", "total", "written"},
			Filters: []binder.Filter{
				{Name: "partner", Column: "partner", Kind: binder.KindText},
				{Name: "tenant", Column: "tenant", Kind: binder.KindText},
				{Name: "period", Column: "period", Kind: binder.KindText},
			},
			Orders: map[string]binder.Order{
				"newest": {Column: "written", Desc: true},
				"oldest": {Column: "written"},
			},
			DefaultOrder: []binder.Order{{Column: "written", Desc: true}},
			PageSize:     100,
			MaxRows:      500,
		},
	},
	{
		Name:        "deliveries-by-partner",
		Title:       "Partner notifications",
		Description: "One row per attempted notification, newest first. Filter by partner or status.",
		Declaration: binder.Declaration{
			Table:   "deliveries",
			Columns: []string{"at", "id", "partner", "endpoint", "status", "attempts"},
			Filters: []binder.Filter{
				{Name: "partner", Column: "partner", Kind: binder.KindText},
				{Name: "status", Column: "status", Kind: binder.KindText},
				{Name: "since", Column: "at", Kind: binder.KindTime, Op: binder.OpGe},
			},
			Orders: map[string]binder.Order{
				"newest": {Column: "at", Desc: true},
				"oldest": {Column: "at"},
			},
			DefaultOrder: []binder.Order{{Column: "at", Desc: true}},
			PageSize:     100,
			MaxRows:      500,
		},
	},
}

// Catalog lists the reports in declaration order.
func Catalog() []Definition { return append([]Definition(nil), definitions...) }

// Lookup resolves a report by its exact name. The comparison is a string comparison, so "Events-By-
// Carrier" is not a report: a name that is not in the catalog is refused instead of being matched to
// the nearest one.
func Lookup(name string) (Definition, error) {
	for _, definition := range definitions {
		if definition.Name == name {
			return definition, nil
		}
	}
	return Definition{}, fmt.Errorf("%w: %q", ErrUnknownReport, name)
}

// Names lists the report names, in declaration order.
func Names() []string {
	names := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		names = append(names, definition.Name)
	}
	return names
}

// Declaration returns the report's filled-in selection for a request.
func (d Definition) Query(values url.Values) (binder.Query, error) {
	return d.Declaration.Query(values)
}

// FilterNames lists the parameters the report accepts, in declaration order, plus the page parameters.
func (d Definition) FilterNames() []string {
	names := make([]string, 0, len(d.Declaration.Filters))
	for _, filter := range d.Declaration.Filters {
		names = append(names, filter.Name)
	}
	return names
}

// OrderNames lists the ordering names the report accepts, sorted.
func (d Definition) OrderNames() []string {
	names := make([]string, 0, len(d.Declaration.Orders))
	for name := range d.Declaration.Orders {
		names = append(names, name)
	}
	sortStrings(names)
	return names
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}
