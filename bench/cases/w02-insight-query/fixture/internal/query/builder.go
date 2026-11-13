// Package query builds the parameterized statements the reports run. A statement carries its values
// as arguments, so a value never becomes part of the statement text.
package query

import (
	"fmt"
	"regexp"
	"strings"
)

// Stmt is one statement and the values bound to it.
type Stmt struct {
	Text string
	Args []any
}

// identifierRe is the shape a table or column name must have. A name that fails it is refused,
// because a name is the one part of a statement the engine cannot quote.
var identifierRe = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

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

// Select starts a statement over the columns given.
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
	b.conds = append(b.conds, column+" = ?")
	b.args = append(b.args, value)
	return b
}

// WhereOp adds a comparison with the operator given. Only the operators the engine implements are
// accepted, so a caller cannot build a statement the parser would refuse at run time.
func (b *Builder) WhereOp(column, op string, value any) *Builder {
	switch op {
	case "=", "!=", "<", "<=", ">", ">=":
	default:
		panic(fmt.Sprintf("query: %q is not an operator", op))
	}
	b.conds = append(b.conds, column+" "+op+" ?")
	b.args = append(b.args, value)
	return b
}

// WhereIn adds an IN condition over the values given.
func (b *Builder) WhereIn(column string, values []any) *Builder {
	if len(values) == 0 {
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

// OrderBy sets the ordering column and direction.
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

// Stmt renders the statement. Every name is checked here, once, so nothing that reaches the engine
// is a name the engine has not been promised.
func (b *Builder) Stmt() (Stmt, error) {
	if len(b.columns) == 0 {
		return Stmt{}, fmt.Errorf("query: no columns selected")
	}
	if !identifierRe.MatchString(b.table) {
		return Stmt{}, fmt.Errorf("query: %q is not a table name", b.table)
	}
	for _, col := range b.columns {
		if !identifierRe.MatchString(col) {
			return Stmt{}, fmt.Errorf("query: %q is not a column name", col)
		}
	}
	if b.order != "" && !identifierRe.MatchString(b.order) {
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
	return Stmt{Text: text.String(), Args: append([]any(nil), b.args...)}, nil
}
