package query

import (
	"fmt"
	"strings"
)

// Filter is one condition a report accepts from a caller: the parameter name it is read from, the
// column it narrows and the operator it applies.
type Filter struct {
	// Param is the query-string parameter the value is read from.
	Param string
	// Column is the column the value is compared with.
	Column string
	// Op is the comparison operator.
	Op string
	// Values lists the accepted values, empty when any value is accepted.
	Values []string
}

// Validate checks the filter declaration.
func (f Filter) Validate() error {
	if f.Param == "" {
		return fmt.Errorf("query: a filter needs a parameter name")
	}
	if !ValidName(f.Param) {
		return fmt.Errorf("query: filter parameter %q is not an identifier", f.Param)
	}
	if !ValidName(f.Column) {
		return fmt.Errorf("query: filter %q names column %q, which is not an identifier", f.Param, f.Column)
	}
	if !validOperator(f.Op) {
		return fmt.Errorf("query: filter %q carries operator %q", f.Param, f.Op)
	}
	for _, value := range f.Values {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("query: filter %q carries an empty accepted value", f.Param)
		}
	}
	return nil
}

// Accepts reports whether value is one this filter takes. A filter with no accepted values takes
// any value; a filter with a list refuses everything outside it.
func (f Filter) Accepts(value string) bool {
	if len(f.Values) == 0 {
		return true
	}
	for _, accepted := range f.Values {
		if accepted == value {
			return true
		}
	}
	return false
}

// Apply binds one value to the statement under this filter. The value is checked against the
// accepted list and then bound as an argument, never written into the statement.
func (f Filter) Apply(b *Builder, value string) error {
	if !f.Accepts(value) {
		return &FilterError{Param: f.Param, Value: value, Accepted: f.Values}
	}
	b.WhereOp(f.Column, f.Op, value)
	return nil
}

// Read applies every filter whose parameter the request carries, and refuses a parameter the report
// does not declare. A parameter that is refused rather than ignored is the difference between a
// narrow report and a report the client believes is narrow.
func Read(b *Builder, filters []Filter, values map[string]string) error {
	declared := make(map[string]Filter, len(filters))
	for _, filter := range filters {
		if err := filter.Validate(); err != nil {
			return err
		}
		declared[filter.Param] = filter
	}
	for _, param := range SortedKeys(values) {
		value := values[param]
		if value == "" {
			continue
		}
		filter, ok := declared[param]
		if !ok {
			return &UndeclaredError{Param: param}
		}
		if err := filter.Apply(b, value); err != nil {
			return err
		}
	}
	return nil
}

// FilterError is the refusal of a value a filter does not accept.
type FilterError struct {
	Param    string
	Value    string
	Accepted []string
}

// Error renders the refusal with the accepted values, so a client can correct its request.
func (e *FilterError) Error() string {
	return fmt.Sprintf("query: filter %q does not accept %q (accepted: %s)",
		e.Param, e.Value, strings.Join(e.Accepted, ", "))
}

// UndeclaredError is the refusal of a parameter no filter declares.
type UndeclaredError struct{ Param string }

// Error renders the refusal.
func (e *UndeclaredError) Error() string {
	return fmt.Sprintf("query: %q is not a filter of this report", e.Param)
}
