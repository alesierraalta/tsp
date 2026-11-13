// This file holds the help and validation a report definition offers around its declaration: the
// parameter names it accepts, the orders it allows and the text a report page shows an operator. The
// declaration itself and the request that fills it in live in query.go, so a reader finds the types and
// the rules in one place.
package binder

import (
	"fmt"
	"sort"
	"strings"
)

// FilterNames lists the parameters the declaration accepts, in declaration order.
func (d Declaration) FilterNames() []string {
	names := make([]string, 0, len(d.Filters))
	for _, filter := range d.Filters {
		names = append(names, filter.Name)
	}
	return names
}

// OrderNames lists the ordering names the declaration accepts, sorted.
func (d Declaration) OrderNames() []string {
	names := make([]string, 0, len(d.Orders))
	for name := range d.Orders {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Reserved reports whether a parameter name is one the binder reads itself. A definition that declares a
// filter by a reserved name would shadow the page bound, so Validate refuses it.
func Reserved(name string) bool {
	for _, reserved := range Defaults {
		if name == reserved {
			return true
		}
	}
	return false
}

// Validate checks the declaration itself, before a request fills it in: the table must be declared, the
// columns and filter columns must exist in it, the orders must name declared columns and no filter may
// take a reserved parameter name. A definition that does not validate is a report that cannot run.
func (d Declaration) Validate() error {
	table, ok := TableOf(d.Table)
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownTable, d.Table)
	}
	if len(d.Columns) == 0 {
		return fmt.Errorf("%w: %q declares no columns", ErrUnknownColumn, d.Table)
	}
	for _, name := range d.Columns {
		if _, ok := table.Column(name); !ok {
			return fmt.Errorf("%w: %q.%q", ErrUnknownColumn, d.Table, name)
		}
	}
	for _, filter := range d.Filters {
		if Reserved(filter.Name) {
			return fmt.Errorf("%w: %q is a page parameter", ErrUnknownFilter, filter.Name)
		}
		if _, ok := table.Column(filter.Column); !ok {
			return fmt.Errorf("%w: %q.%q", ErrUnknownColumn, d.Table, filter.Column)
		}
		if filter.Op != "" {
			if _, err := filter.Op.SQL(); err != nil {
				return err
			}
		}
	}
	for name, order := range d.Orders {
		if name == "" {
			return fmt.Errorf("%w: an order has no name", ErrUnknownOrder)
		}
		if _, ok := table.Column(order.Column); !ok {
			return fmt.Errorf("%w: %q.%q", ErrUnknownColumn, d.Table, order.Column)
		}
	}
	if d.PageSize < 1 || d.PageSize > d.MaxRows || d.MaxRows > table.MaxRows {
		return fmt.Errorf("%w: page %d, max %d, table ceiling %d", ErrLimit, d.PageSize, d.MaxRows, table.MaxRows)
	}
	return nil
}

// Describe renders the declaration for a help page: the table, the columns, the filters with their
// kinds, the orders and the page size. It prints names, never values.
func (d Declaration) Describe() string {
	parts := []string{fmt.Sprintf("table %s", d.Table)}
	parts = append(parts, "columns "+strings.Join(d.Columns, ", "))
	for _, filter := range d.Filters {
		op := filter.Op
		if op == "" {
			op = OpEq
		}
		parts = append(parts, fmt.Sprintf("?%s: %s %s", filter.Name, filter.Kind, op))
	}
	if names := d.OrderNames(); len(names) > 0 {
		parts = append(parts, "order "+strings.Join(names, "|"))
	}
	parts = append(parts, fmt.Sprintf("page %d..%d", 1, d.MaxRows))
	return strings.Join(parts, "; ")
}
