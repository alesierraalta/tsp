// Package query builds the selections the reports and the exports run. A definition fixes the
// table, its columns, the filters it accepts and the orders it allows. A caller's parameter is
// looked up in that definition: an undeclared parameter is refused, a declared one is parsed by its
// kind, and its value is carried as a bound argument. The statement an export reports is rendered
// from the definition with "?" in place of every value and is never parsed back, so the text
// describes the selection and does not make it.
package query

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Errors the builder reports.
var (
	ErrUnknown = errors.New("query: unknown parameter")
	ErrValue   = errors.New("query: invalid value")
	ErrLimit   = errors.New("query: limit out of range")
	ErrInvalid = errors.New("query: invalid definition")
)

// Kind is the type a parameter's value is parsed as.
type Kind int

// The kinds a parameter may have.
const (
	KindText Kind = iota
	KindEnum
)

// identRE is the form of an identifier a definition declares. Every name in a statement comes from
// the definition, and this rule is what makes the statement text a template rather than a
// concatenation of whatever a caller sent.
var identRE = regexp.MustCompile(`^[a-z][a-z0-9_]{0,40}$`)

// Param is one declared filter.
type Param struct {
	Name   string
	Column string
	Kind   Kind
	Enum   []string
}

// Definition is one selectable relation.
type Definition struct {
	Table        string
	Columns      []string
	Params       []Param
	Sorts        map[string]string
	MaxLimit     int
	DefaultLimit int
}

// Validate refuses a definition that could not produce a safe statement: an identifier that is not
// one, a column a filter or a sort names but the relation does not carry, an enumerated parameter
// with no values, or a limit outside 1..MaxLimit.
func (d Definition) Validate() error {
	if !identRE.MatchString(d.Table) || len(d.Columns) == 0 {
		return fmt.Errorf("%w: table %q", ErrInvalid, d.Table)
	}
	columns := make(map[string]bool, len(d.Columns))
	for _, column := range d.Columns {
		if !identRE.MatchString(column) || columns[column] {
			return fmt.Errorf("%w: column %q", ErrInvalid, column)
		}
		columns[column] = true
	}
	names := make(map[string]bool, len(d.Params))
	for _, param := range d.Params {
		if !identRE.MatchString(param.Name) || !identRE.MatchString(param.Column) || !columns[param.Column] {
			return fmt.Errorf("%w: parameter %q", ErrInvalid, param.Name)
		}
		if param.Kind < KindText || param.Kind > KindEnum || (param.Kind == KindEnum && len(param.Enum) == 0) {
			return fmt.Errorf("%w: parameter %q kind", ErrInvalid, param.Name)
		}
		if names[param.Name] {
			return fmt.Errorf("%w: parameter %q twice", ErrInvalid, param.Name)
		}
		names[param.Name] = true
	}
	for name, column := range d.Sorts {
		if !identRE.MatchString(name) || !columns[column] {
			return fmt.Errorf("%w: sort %q -> %q", ErrInvalid, name, column)
		}
	}
	if d.MaxLimit <= 0 || d.DefaultLimit <= 0 || d.DefaultLimit > d.MaxLimit {
		return fmt.Errorf("%w: limits %d/%d", ErrInvalid, d.DefaultLimit, d.MaxLimit)
	}
	return nil
}

// Clause is one comparison: a declared column and a value carried as it was parsed.
type Clause struct {
	Column string
	Value  any
}

// Sort is one ordering: a declared column and a direction.
type Sort struct {
	Column string
	Desc   bool
}

// Criteria is what a builder produced. A store applies it by looking each column up in its own switch, so
// the definition is the allowlist and the store is the second check. Accounts is the caller's scope: the
// account ids the caller may read. An empty scope reads nothing, and the scope is applied before the page
// is cut, so another tenant's rows never spend it.
type Criteria struct {
	Clauses  []Clause
	Sorts    []Sort
	Accounts []string
	Limit    int
}

// Builder accumulates the selection a definition allows.
type Builder struct {
	definition Definition
	params     map[string]Param
	clauses    []Clause
	sorts      []Sort
	limit      int
}

// New returns a builder for a definition.
func New(definition Definition) (*Builder, error) {
	if err := definition.Validate(); err != nil {
		return nil, err
	}
	params := make(map[string]Param, len(definition.Params))
	for _, param := range definition.Params {
		params[param.Name] = param
	}
	return &Builder{definition: definition, params: params, limit: definition.DefaultLimit}, nil
}

// Filter adds one declared filter. A name the definition does not carry is refused; a value that
// does not parse as the parameter's kind is refused; nothing a caller sent reaches the statement as
// text, because the value is stored and the statement holds a placeholder for it.
func (b *Builder) Filter(name, value string) error {
	param, ok := b.params[name]
	if !ok {
		return fmt.Errorf("%w: %q on %s", ErrUnknown, name, b.definition.Table)
	}
	parsed, err := parseValue(param, value)
	if err != nil {
		return err
	}
	b.clauses = append(b.clauses, Clause{Column: param.Column, Value: parsed})
	return nil
}

// parseValue parses one value by its kind.
func parseValue(param Param, value string) (any, error) {
	switch param.Kind {
	case KindText:
		if len(value) > 200 {
			return nil, fmt.Errorf("%w: %q is longer than 200 bytes", ErrValue, param.Name)
		}
		return value, nil
	case KindEnum:
		for _, allowed := range param.Enum {
			if allowed == value {
				return value, nil
			}
		}
		return nil, fmt.Errorf("%w: %q is not one of %s", ErrValue, value, strings.Join(param.Enum, ","))
	default:
		return nil, fmt.Errorf("%w: parameter %q", ErrValue, param.Name)
	}
}

// Sort adds one ordering. The requested name is looked up in the definition's table: the column
// that reaches the statement is the one the definition carries, never the string the caller sent.
func (b *Builder) Sort(name string, desc bool) error {
	column, ok := b.definition.Sorts[name]
	if !ok {
		return fmt.Errorf("%w: order %q on %s", ErrUnknown, name, b.definition.Table)
	}
	b.sorts = append(b.sorts, Sort{Column: column, Desc: desc})
	return nil
}

// Limit sets the page. A page above the definition's cap is refused rather than clamped, because a
// caller that asked for more rows than a report serves has made a mistake worth naming.
func (b *Builder) Limit(rows int) error {
	if rows <= 0 || rows > b.definition.MaxLimit {
		return fmt.Errorf("%w: %d is not in 1..%d", ErrLimit, rows, b.definition.MaxLimit)
	}
	b.limit = rows
	return nil
}

// Criteria is the selection the builder holds.
func (b *Builder) Criteria() Criteria {
	return Criteria{
		Clauses: append([]Clause(nil), b.clauses...),
		Sorts:   append([]Sort(nil), b.sorts...),
		Limit:   b.limit,
	}
}

// Params lists the filters the definition accepts, sorted by name, which is what a report endpoint
// publishes so a caller can see what it may ask.
func (b *Builder) Params() []Param {
	out := make([]Param, 0, len(b.params))
	for _, param := range b.params {
		out = append(out, param)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Statement renders the selection as text with a placeholder for every value, and the arguments in
// the order the placeholders appear. It is what an export records as the selection it ran; the
// values are returned beside it and are never interpolated into it.
func (b *Builder) Statement() (string, []any) {
	var text strings.Builder
	text.WriteString("SELECT " + strings.Join(b.definition.Columns, ", ") + " FROM " + b.definition.Table)
	args := make([]any, 0, len(b.clauses)+1)
	if len(b.clauses) > 0 {
		text.WriteString(" WHERE ")
		for i, clause := range b.clauses {
			if i > 0 {
				text.WriteString(" AND ")
			}
			text.WriteString(clause.Column + " = ?")
			args = append(args, clause.Value)
		}
	}
	if len(b.sorts) > 0 {
		text.WriteString(" ORDER BY ")
		for i, sort := range b.sorts {
			if i > 0 {
				text.WriteString(", ")
			}
			text.WriteString(sort.Column)
			if sort.Desc {
				text.WriteString(" DESC")
			} else {
				text.WriteString(" ASC")
			}
		}
	}
	text.WriteString(" LIMIT ?")
	args = append(args, b.limit)
	return text.String(), args
}
