// Package query builds the parameterized statements the reports run. A statement carries its values
// as bound arguments, so a value never becomes part of the statement text and the engine that parses
// the text never sees anything but a placeholder where the value goes.
package query

import (
	"fmt"
	"sort"
	"strings"
)

// Stmt is one statement and the values bound to it, in the order the placeholders appear.
type Stmt struct {
	Text string
	Args []any
}

// Builder assembles one SELECT.
type Builder struct {
	columns  []string
	table    string
	conds    []string
	args     []any
	order    string
	desc     bool
	limit    int
	distinct bool
}

// Select starts a statement over the columns given. The columns are checked when the statement is
// rendered, so a caller that builds a list of names from a definition still gets a refusal.
func Select(columns ...string) *Builder {
	return &Builder{columns: append([]string(nil), columns...)}
}

// From names the table.
func (b *Builder) From(table string) *Builder {
	b.table = table
	return b
}

// Distinct selects distinct rows.
func (b *Builder) Distinct() *Builder {
	b.distinct = true
	return b
}

// Where adds an equality condition. The column name is validated; the value travels as an argument.
func (b *Builder) Where(column string, value any) *Builder {
	return b.WhereOp(column, "=", value)
}

// WhereOp adds a comparison with the operator given. Only the operators the engine implements are
// accepted, so a caller cannot build a statement the parser would refuse at run time, and an
// operator is never read from a request.
func (b *Builder) WhereOp(column, op string, value any) *Builder {
	if !validOperator(op) {
		panic(fmt.Sprintf("query: %q is not an operator", op))
	}
	b.conds = append(b.conds, column+" "+op+" ?")
	b.args = append(b.args, value)
	return b
}

// WhereIn adds an IN condition over the values given.
func (b *Builder) WhereIn(column string, values []any) *Builder {
	if len(values) == 0 {
		// An empty IN matches nothing; an empty list would otherwise render as "IN ()" and be a
		// statement the parser refuses.
		b.conds = append(b.conds, "1 = 0")
		return b
	}
	marks := make([]string, 0, len(values))
	for range values {
		marks = append(marks, "?")
	}
	b.conds = append(b.conds, column+" IN ("+strings.Join(marks, ", ")+")")
	b.args = append(b.args, values...)
	return b
}

// WhereNotEqual adds an inequality condition.
func (b *Builder) WhereNotEqual(column string, value any) *Builder {
	return b.WhereOp(column, "!=", value)
}

// WhereBetween adds a half-open range condition: from is inside it, to is not.
func (b *Builder) WhereBetween(column string, from, to any) *Builder {
	return b.WhereOp(column, ">=", from).WhereOp(column, "<", to)
}

// OrderBy sets the ordering column and direction. The default direction is ascending, which is what
// a listing a client pages through needs to be stable under.
func (b *Builder) OrderBy(column string, desc bool) *Builder {
	b.order = column
	b.desc = desc
	return b
}

// Limit bounds the rows returned. A non-positive limit is refused rather than dropped, because a
// report without a bound is the query that reads a whole table.
func (b *Builder) Limit(n int) error {
	if n <= 0 {
		return fmt.Errorf("query: limit %d is not positive", n)
	}
	b.limit = n
	return nil
}

// Clone returns a copy a caller can extend without changing the original, which is how a report
// definition stays the definition while one route narrows it.
func (b *Builder) Clone() *Builder {
	return &Builder{
		columns:  append([]string(nil), b.columns...),
		table:    b.table,
		conds:    append([]string(nil), b.conds...),
		args:     append([]any(nil), b.args...),
		order:    b.order,
		desc:     b.desc,
		limit:    b.limit,
		distinct: b.distinct,
	}
}

// Args returns a copy of the values bound so far, in placeholder order.
func (b *Builder) Args() []any {
	return append([]any(nil), b.args...)
}

// Stmt renders the statement. Every name is checked here, once, so nothing that reaches the engine
// is a name the engine has not been promised.
func (b *Builder) Stmt() (Stmt, error) {
	if len(b.columns) == 0 {
		return Stmt{}, fmt.Errorf("query: no columns selected")
	}
	if !ValidName(b.table) {
		return Stmt{}, fmt.Errorf("query: %q is not a table name", b.table)
	}
	for _, column := range b.columns {
		if !ValidName(column) {
			return Stmt{}, fmt.Errorf("query: %q is not a column name", column)
		}
	}
	if b.order != "" && !ValidName(b.order) {
		return Stmt{}, fmt.Errorf("query: %q is not an ordering column", b.order)
	}
	var text strings.Builder
	text.WriteString("SELECT ")
	if b.distinct {
		text.WriteString("DISTINCT ")
	}
	text.WriteString(strings.Join(b.columns, ", "))
	text.WriteString(" FROM ")
	text.WriteString(b.table)
	if len(b.conds) > 0 {
		text.WriteString(" WHERE ")
		text.WriteString(strings.Join(b.conds, " AND "))
	}
	if b.order != "" {
		text.WriteString(" ORDER BY ")
		text.WriteString(b.order)
		if b.desc {
			text.WriteString(" DESC")
		}
	}
	if b.limit > 0 {
		fmt.Fprintf(&text, " LIMIT %d", b.limit)
	}
	return Stmt{Text: text.String(), Args: b.Args()}, nil
}

// validOperator reports whether op is one the engine implements.
func validOperator(op string) bool {
	switch op {
	case "=", "!=", "<", "<=", ">", ">=":
		return true
	}
	return false
}

// SortedKeys returns the keys of a map in order, for a caller that renders a definition or a set of
// filter names and needs the output to be the same on every run.
func SortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
