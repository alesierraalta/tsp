// Package binder is how a report becomes a statement. A caller describes a selection — a table, the
// columns to read, the filters and the order — and this package validates every name in it against a
// declared table and renders SQL text in which every value is a `?` placeholder with a bound argument.
// No value a request supplies is ever concatenated into the text, and no name reaches the text that the
// table does not declare, so the SQL a report runs is the SQL its definition describes.
package binder

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Errors a selection reports.
var (
	ErrUnknownTable  = errors.New("binder: unknown table")
	ErrUnknownColumn = errors.New("binder: unknown column")
	ErrUnknownFilter = errors.New("binder: undeclared parameter")
	ErrUnknownOrder  = errors.New("binder: unknown order")
	ErrBadValue      = errors.New("binder: value does not fit its column")
	ErrLimit         = errors.New("binder: limit outside the definition")
)

// Kind is the type of a column's values. It decides how a filter value is parsed and how two values
// are compared, and it is why an integer filter cannot arrive as text: the parse happens before the
// value is bound.
type Kind int

// The kinds a column may carry.
const (
	KindText Kind = iota
	KindInt
	KindTime
	KindMoney
	KindBool
)

// String names a kind for a page and a test.
func (k Kind) String() string {
	switch k {
	case KindText:
		return "text"
	case KindInt:
		return "integer"
	case KindTime:
		return "instant"
	case KindMoney:
		return "money"
	case KindBool:
		return "boolean"
	default:
		return "unknown"
	}
}

// Column is one declared column of a table.
type Column struct {
	Name string
	Kind Kind
}

// Table is one declared table: its name, its columns and the most rows a selection may read from it.
type Table struct {
	Name     string
	Columns  []Column
	MaxRows  int
	OrderKey string
}

// Column resolves a column by its exact declared name.
func (t Table) Column(name string) (Column, bool) {
	for _, column := range t.Columns {
		if column.Name == name {
			return column, true
		}
	}
	return Column{}, false
}

// Names lists the table's column names in declared order.
func (t Table) Names() []string {
	names := make([]string, 0, len(t.Columns))
	for _, column := range t.Columns {
		names = append(names, column.Name)
	}
	return names
}

// catalog is the declared schema. A table or a column that is not here cannot reach a selection: the
// store refuses it and so does the renderer, which is what keeps a column name from being a string a
// request supplies.
var catalog = []Table{
	{
		Name: "events",
		Columns: []Column{
			{Name: "id", Kind: KindText},
			{Name: "tenant", Kind: KindText},
			{Name: "partner", Kind: KindText},
			{Name: "shipment", Kind: KindText},
			{Name: "carrier", Kind: KindText},
			{Name: "lane", Kind: KindText},
			{Name: "service", Kind: KindText},
			{Name: "kind", Kind: KindText},
			{Name: "at", Kind: KindTime},
			{Name: "facility", Kind: KindText},
			{Name: "grams", Kind: KindInt},
			{Name: "recorded", Kind: KindTime},
		},
		MaxRows:  5000,
		OrderKey: "at",
	},
	{
		Name: "shipments",
		Columns: []Column{
			{Name: "id", Kind: KindText},
			{Name: "tenant", Kind: KindText},
			{Name: "partner", Kind: KindText},
			{Name: "carrier", Kind: KindText},
			{Name: "lane", Kind: KindText},
			{Name: "service", Kind: KindText},
			{Name: "grams", Kind: KindInt},
			{Name: "created", Kind: KindTime},
			{Name: "status", Kind: KindText},
		},
		MaxRows:  5000,
		OrderKey: "created",
	},
	{
		Name: "statements",
		Columns: []Column{
			{Name: "id", Kind: KindText},
			{Name: "tenant", Kind: KindText},
			{Name: "partner", Kind: KindText},
			{Name: "period", Kind: KindText},
			{Name: "lines", Kind: KindInt},
			{Name: "net", Kind: KindMoney},
			{Name: "tax", Kind: KindMoney},
			{Name: "total", Kind: KindMoney},
			{Name: "written", Kind: KindTime},
		},
		MaxRows:  1000,
		OrderKey: "written",
	},
	{
		Name: "deliveries",
		Columns: []Column{
			{Name: "id", Kind: KindText},
			{Name: "partner", Kind: KindText},
			{Name: "endpoint", Kind: KindText},
			{Name: "status", Kind: KindText},
			{Name: "attempts", Kind: KindInt},
			{Name: "at", Kind: KindTime},
		},
		MaxRows:  1000,
		OrderKey: "at",
	},
}

// Catalog lists the declared tables.
func Catalog() []Table { return append([]Table(nil), catalog...) }

// TableOf resolves a table by its exact name.
func TableOf(name string) (Table, bool) {
	for _, table := range catalog {
		if table.Name == name {
			return table, true
		}
	}
	return Table{}, false
}

// Filter is one parameter a definition accepts, and the column it compares. A filter's kind is its
// column's kind, so the value a request sends is parsed into the type the column holds before it is
// bound; the operator is declared too, which is how a "since" filter is a `>=` on an instant without
// the request naming an operator.
type Filter struct {
	Name   string
	Column string
	Kind   Kind
	Op     Op
}

// Declaration is the part of a report definition that decides what a request may ask for: the table it
// reads, the columns it selects, the filters it accepts, the orders it allows and the page it returns.
// A report is a declaration; a request fills it in and can never add to it.
type Declaration struct {
	Table        string
	Columns      []string
	Filters      []Filter
	Orders       map[string]Order
	DefaultOrder []Order
	PageSize     int
	MaxRows      int
}

// Op is a comparison a filter applies.
type Op string

// The comparisons a filter may use. The set is closed; an operator the table does not name is refused
// rather than passed to the engine, which would make the operator a piece of SQL text a caller chose.
const (
	OpEq Op = "eq"
	OpNe Op = "ne"
	OpLt Op = "lt"
	OpLe Op = "le"
	OpGt Op = "gt"
	OpGe Op = "ge"
)

// ParseOp reads an operator exactly.
func ParseOp(raw string) (Op, error) {
	switch Op(raw) {
	case OpEq, OpNe, OpLt, OpLe, OpGt, OpGe:
		return Op(raw), nil
	}
	return "", fmt.Errorf("%w: operator %q", ErrBadValue, raw)
}

// SQL is the operator's SQL spelling. It is a declared constant, never a value a caller supplied.
func (o Op) SQL() (string, error) {
	switch o {
	case OpEq:
		return "=", nil
	case OpNe:
		return "<>", nil
	case OpLt:
		return "<", nil
	case OpLe:
		return "<=", nil
	case OpGt:
		return ">", nil
	case OpGe:
		return ">=", nil
	}
	return "", fmt.Errorf("%w: operator %q", ErrBadValue, o)
}

// Pred is one filter of a selection.
type Pred struct {
	Column string
	Op     Op
	Value  any
}

// Order is one ordering term of a selection.
type Order struct {
	Column string
	Desc   bool
}

// Query is a selection, before it is rendered or run.
type Query struct {
	Table   string
	Columns []string
	Preds   []Pred
	Order   []Order
	Limit   int
}

// Row is one selected row. A value's Go type follows its column's kind: string, int64, time.Time or
// bool.
type Row map[string]any

// identRE is the form of an identifier this package will quote. It is anchored and starts with a
// letter, and a name that does not match is refused before quoting; the declared lookup is the real
// guard, and this rule is what keeps a quoted name from carrying a quote of its own.
var identRE = func(name string) bool {
	if name == "" || len(name) > 32 {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c >= 'a' && c <= 'z' || c == '_' {
			continue
		}
		if i == 0 {
			return false
		}
		if c >= '0' && c <= '9' {
			continue
		}
		return false
	}
	return true
}

// Quote wraps a declared identifier in double quotes. It is called only with a name this package has
// already matched against a table's declared columns, and the form check is repeated here so the
// function is safe on its own as well.
func Quote(name string) (string, error) {
	if !identRE(name) {
		return "", fmt.Errorf("%w: %q is not an identifier", ErrUnknownColumn, name)
	}
	return `"` + name + `"`, nil
}

// Validate checks every name and value in the selection against the declared table.
func (q Query) Validate() error {
	table, ok := TableOf(q.Table)
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownTable, q.Table)
	}
	if len(q.Columns) == 0 {
		return fmt.Errorf("%w: %q has no columns", ErrUnknownColumn, q.Table)
	}
	for _, name := range q.Columns {
		if _, ok := table.Column(name); !ok {
			return fmt.Errorf("%w: %q.%q", ErrUnknownColumn, q.Table, name)
		}
	}
	for _, pred := range q.Preds {
		column, ok := table.Column(pred.Column)
		if !ok {
			return fmt.Errorf("%w: %q.%q", ErrUnknownColumn, q.Table, pred.Column)
		}
		if _, err := pred.Op.SQL(); err != nil {
			return err
		}
		if err := checkValue(column, pred.Value); err != nil {
			return err
		}
	}
	for _, order := range q.Order {
		if _, ok := table.Column(order.Column); !ok {
			return fmt.Errorf("%w: %q.%q", ErrUnknownColumn, q.Table, order.Column)
		}
	}
	if q.Limit < 0 || q.Limit > table.MaxRows {
		return fmt.Errorf("%w: %d is outside 1..%d", ErrLimit, q.Limit, table.MaxRows)
	}
	return nil
}

// checkValue refuses a value whose Go type does not fit its column's kind. A text value cannot be
// bound to an integer column by accident, and a nil value is refused rather than rendered as NULL the
// caller did not ask for.
func checkValue(column Column, value any) error {
	switch column.Kind {
	case KindText:
		if _, ok := value.(string); !ok {
			return fmt.Errorf("%w: %q wants text, got %T", ErrBadValue, column.Name, value)
		}
	case KindInt, KindMoney:
		if _, ok := value.(int64); !ok {
			return fmt.Errorf("%w: %q wants an integer, got %T", ErrBadValue, column.Name, value)
		}
	case KindTime:
		if _, ok := value.(time.Time); !ok {
			return fmt.Errorf("%w: %q wants an instant, got %T", ErrBadValue, column.Name, value)
		}
	case KindBool:
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%w: %q wants a boolean, got %T", ErrBadValue, column.Name, value)
		}
	default:
		return fmt.Errorf("%w: %q has no kind", ErrBadValue, column.Name)
	}
	return nil
}

// Render writes the SQL text and the bound arguments. The text carries one `?` per predicate and the
// arguments are returned beside it, in the same order: the values never enter the text, so a value
// that looks like SQL is a value. A limit of zero means the table's ceiling, which is a bound the
// engine applies, not a clamp of a caller's larger number.
func (q Query) Render() (string, []any, error) {
	if err := q.Validate(); err != nil {
		return "", nil, err
	}
	table, _ := TableOf(q.Table)
	var text strings.Builder
	text.WriteString("SELECT ")
	for i, name := range q.Columns {
		if i > 0 {
			text.WriteString(", ")
		}
		quoted, err := Quote(name)
		if err != nil {
			return "", nil, err
		}
		text.WriteString(quoted)
	}
	quotedTable, err := Quote(table.Name)
	if err != nil {
		return "", nil, err
	}
	text.WriteString(" FROM ")
	text.WriteString(quotedTable)
	args := make([]any, 0, len(q.Preds))
	if len(q.Preds) > 0 {
		text.WriteString(" WHERE ")
		for i, pred := range q.Preds {
			if i > 0 {
				text.WriteString(" AND ")
			}
			quoted, err := Quote(pred.Column)
			if err != nil {
				return "", nil, err
			}
			op, err := pred.Op.SQL()
			if err != nil {
				return "", nil, err
			}
			text.WriteString(quoted)
			text.WriteByte(' ')
			text.WriteString(op)
			text.WriteString(" ?")
			args = append(args, pred.Value)
		}
	}
	if len(q.Order) > 0 {
		text.WriteString(" ORDER BY ")
		for i, order := range q.Order {
			if i > 0 {
				text.WriteString(", ")
			}
			quoted, err := Quote(order.Column)
			if err != nil {
				return "", nil, err
			}
			text.WriteString(quoted)
			if order.Desc {
				text.WriteString(" DESC")
			}
		}
	}
	limit := q.Limit
	if limit <= 0 {
		limit = table.MaxRows
	}
	text.WriteString(" LIMIT ")
	text.WriteString(fmt.Sprint(limit))
	return text.String(), args, nil
}

// Describe renders the selection for a page: the table, the columns and the filters as text. The
// values are described, never inlined into SQL, which is why a description is safe to print.
func (q Query) Describe() string {
	parts := []string{q.Table, "[" + strings.Join(q.Columns, ", ") + "]"}
	for _, pred := range q.Preds {
		parts = append(parts, fmt.Sprintf("%s %s ?", pred.Column, pred.Op))
	}
	if len(q.Order) > 0 {
		terms := make([]string, 0, len(q.Order))
		for _, order := range q.Order {
			term := order.Column
			if order.Desc {
				term += " desc"
			}
			terms = append(terms, term)
		}
		parts = append(parts, "order "+strings.Join(terms, ", "))
	}
	parts = append(parts, fmt.Sprintf("limit %d", q.Limit))
	return strings.Join(parts, " ")
}

// SortedColumns returns the selection's columns sorted, for a stable test.
func (q Query) SortedColumns() []string {
	names := append([]string(nil), q.Columns...)
	sort.Strings(names)
	return names
}
