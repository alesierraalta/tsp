package report

import (
	"fmt"
	"net/url"
	"strconv"
	"time"

	"example.com/shiptrack/internal/binder"
	"example.com/shiptrack/internal/money"
	"example.com/shiptrack/internal/store"
)

// Result is a report that ran: the rows it selected, the columns it shows, and the SQL text and bound
// arguments it ran with. The text carries `?` in place of every value, so it is safe to print next to
// the report and it is what an export header says the report selected.
type Result struct {
	Name      string
	Title     string
	Columns   []string
	Rows      []binder.Row
	SQL       string
	Args      []any
	Selection string
}

// Run renders the report's selection from a request and runs it over the store. The order is fixed:
// the request is read into a selection, the selection is validated, and only then does the store see a
// row. A refused parameter therefore costs no read.
func (d Definition) Run(st *store.Store, values url.Values) (Result, error) {
	query, err := d.Declaration.Query(values)
	if err != nil {
		return Result{}, err
	}
	text, args, err := query.Render()
	if err != nil {
		return Result{}, err
	}
	rows, err := st.Select(query)
	if err != nil {
		return Result{}, err
	}
	return Result{
		Name:      d.Name,
		Title:     d.Title,
		Columns:   append([]string(nil), query.Columns...),
		Rows:      rows,
		SQL:       text,
		Args:      args,
		Selection: query.Describe(),
	}, nil
}

// Kind is the kind of a column of the result, from the report's declared table.
func (r Result) Kind(column string) binder.Kind {
	definition, err := Lookup(r.Name)
	if err != nil {
		return binder.KindText
	}
	table, ok := binder.TableOf(definition.Declaration.Table)
	if !ok {
		return binder.KindText
	}
	declared, ok := table.Column(column)
	if !ok {
		return binder.KindText
	}
	return declared.Kind
}

// Numeric reports whether a column may be rendered as a number. A money or integer column is numeric;
// in an export a numeric cell may begin with a minus sign and a text cell may not.
func (r Result) Numeric(column string) bool {
	kind := r.Kind(column)
	return kind == binder.KindInt || kind == binder.KindMoney
}

// Cell renders a row's value in a column as the text a page and an export show. An amount is rendered
// from its integer minor units, an instant in UTC, and a missing value as an empty cell — never as
// "<nil>".
func (r Result) Cell(row binder.Row, column string) string {
	value, ok := row[column]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case int64:
		if r.Kind(column) == binder.KindMoney {
			return money.Amount(typed).String()
		}
		return strconv.FormatInt(typed, 10)
	case time.Time:
		return typed.UTC().Format(time.RFC3339)
	case bool:
		if typed {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprint(typed)
	}
}

// Cells renders one row as the slice an export writer takes, in the result's column order.
func (r Result) Cells(row binder.Row) []string {
	cells := make([]string, 0, len(r.Columns))
	for _, column := range r.Columns {
		cells = append(cells, r.Cell(row, column))
	}
	return cells
}

// NumericColumns reports which columns are numeric, in the result's column order.
func (r Result) NumericColumns() []bool {
	numeric := make([]bool, 0, len(r.Columns))
	for _, column := range r.Columns {
		numeric = append(numeric, r.Numeric(column))
	}
	return numeric
}
