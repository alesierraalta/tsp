package sqlmem

import (
	"fmt"
	"strconv"
	"strings"
)

// Statement is one parsed SELECT.
type Statement struct {
	Columns []string
	Table   string
	Where   Expr
	Order   string
	Desc    bool
	Limit   int
}

// Expr is a predicate the evaluator can run against a row.
type Expr interface {
	// Match reports whether the row satisfies the expression. A column the row does not carry is
	// an error, never a silent false: a misspelled filter must not read as "no rows".
	Match(row Row) (bool, error)
}

// Row is one decoded row of a table.
type Row map[string]Value

// Value is a cell: text, or an integer. The engine has no other type, which keeps a comparison
// total.
type Value struct {
	Text  string
	Int   int64
	IsInt bool
}

// Text builds a text value.
func Text(s string) Value { return Value{Text: s} }

// Int builds an integer value.
func Int(n int64) Value { return Value{Int: n, IsInt: true} }

// String renders the value the way the JSON encoder hands it out.
func (v Value) String() string {
	if v.IsInt {
		return strconv.FormatInt(v.Int, 10)
	}
	return v.Text
}

// compare reports -1, 0 or 1 for two values, and whether they are comparable at all.
func compare(a, b Value) (int, bool) {
	if a.IsInt && b.IsInt {
		switch {
		case a.Int < b.Int:
			return -1, true
		case a.Int > b.Int:
			return 1, true
		default:
			return 0, true
		}
	}
	if !a.IsInt && !b.IsInt {
		return strings.Compare(a.Text, b.Text), true
	}
	return 0, false
}

type columnExpr struct {
	Column string
	Op     string
	Value  Value
}

func (e columnExpr) Match(row Row) (bool, error) {
	got, ok := row[e.Column]
	if !ok {
		return false, fmt.Errorf("sqlmem: the table has no column %q", e.Column)
	}
	return applyOp(got, e.Op, e.Value)
}

type literalExpr struct {
	Left  Value
	Op    string
	Right Value
}

func (e literalExpr) Match(Row) (bool, error) {
	return applyOp(e.Left, e.Op, e.Right)
}

type andExpr struct{ Left, Right Expr }

func (e andExpr) Match(row Row) (bool, error) {
	ok, err := e.Left.Match(row)
	if err != nil || !ok {
		return false, err
	}
	return e.Right.Match(row)
}

type orExpr struct{ Left, Right Expr }

func (e orExpr) Match(row Row) (bool, error) {
	ok, err := e.Left.Match(row)
	if err != nil {
		return false, err
	}
	if ok {
		return true, nil
	}
	return e.Right.Match(row)
}

func applyOp(a Value, op string, b Value) (bool, error) {
	cmp, ok := compare(a, b)
	if !ok {
		return false, fmt.Errorf("sqlmem: cannot compare %q with %q", a.String(), b.String())
	}
	switch op {
	case "=":
		return cmp == 0, nil
	case "!=":
		return cmp != 0, nil
	case "<":
		return cmp < 0, nil
	case "<=":
		return cmp <= 0, nil
	case ">":
		return cmp > 0, nil
	case ">=":
		return cmp >= 0, nil
	default:
		return false, fmt.Errorf("sqlmem: %q is not an operator", op)
	}
}

type parser struct {
	tokens []Token
	at     int
}

// Parse reads one SELECT statement. Anything the engine does not implement is refused with the
// position it was found at, so a statement that means something else never runs as if it did not.
func Parse(sql string) (Statement, error) {
	tokens, err := Lex(sql)
	if err != nil {
		return Statement{}, err
	}
	p := &parser{tokens: tokens}
	stmt, err := p.statement()
	if err != nil {
		return Statement{}, err
	}
	if p.peek().Kind != EOF {
		return Statement{}, fmt.Errorf("sqlmem: unexpected %q at %d", p.peek().Text, p.peek().Pos)
	}
	return stmt, nil
}

func (p *parser) peek() Token { return p.tokens[p.at] }

func (p *parser) take() Token {
	t := p.tokens[p.at]
	if t.Kind != EOF {
		p.at++
	}
	return t
}

func (p *parser) keyword(word string) bool {
	t := p.peek()
	return t.Kind == Word && strings.EqualFold(t.Text, word)
}

func (p *parser) statement() (Statement, error) {
	var stmt Statement
	if !p.keyword("select") {
		return stmt, fmt.Errorf("sqlmem: only SELECT is supported, found %q", p.peek().Text)
	}
	p.take()
	for {
		t := p.take()
		switch t.Kind {
		case Star:
			stmt.Columns = append(stmt.Columns, "*")
		case Word:
			stmt.Columns = append(stmt.Columns, t.Text)
		default:
			return stmt, fmt.Errorf("sqlmem: %q at %d is not a column", t.Text, t.Pos)
		}
		if p.peek().Kind == Comma {
			p.take()
			continue
		}
		break
	}
	if !p.keyword("from") {
		return stmt, fmt.Errorf("sqlmem: expected FROM, found %q", p.peek().Text)
	}
	p.take()
	table := p.take()
	if table.Kind != Word {
		return stmt, fmt.Errorf("sqlmem: %q at %d is not a table", table.Text, table.Pos)
	}
	stmt.Table = table.Text

	if p.keyword("where") {
		p.take()
		expr, err := p.condition()
		if err != nil {
			return stmt, err
		}
		stmt.Where = expr
	}
	if p.keyword("order") {
		p.take()
		if !p.keyword("by") {
			return stmt, fmt.Errorf("sqlmem: expected BY after ORDER")
		}
		p.take()
		col := p.take()
		if col.Kind != Word {
			return stmt, fmt.Errorf("sqlmem: %q at %d is not a column to order by", col.Text, col.Pos)
		}
		stmt.Order = col.Text
		if p.keyword("desc") {
			p.take()
			stmt.Desc = true
		} else if p.keyword("asc") {
			p.take()
		}
	}
	if p.keyword("limit") {
		p.take()
		n := p.take()
		if n.Kind != Word || !isNumber(n.Text) {
			return stmt, fmt.Errorf("sqlmem: %q at %d is not a row limit", n.Text, n.Pos)
		}
		value, err := strconv.Atoi(n.Text)
		if err != nil || value < 0 {
			return stmt, fmt.Errorf("sqlmem: %q is not a row limit", n.Text)
		}
		stmt.Limit = value
	}
	return stmt, nil
}

// condition reads comparisons joined by AND and OR, evaluated left to right.
func (p *parser) condition() (Expr, error) {
	expr, err := p.comparison()
	if err != nil {
		return nil, err
	}
	for p.keyword("and") || p.keyword("or") {
		op := strings.ToLower(p.take().Text)
		right, err := p.comparison()
		if err != nil {
			return nil, err
		}
		if op == "and" {
			expr = andExpr{Left: expr, Right: right}
		} else {
			expr = orExpr{Left: expr, Right: right}
		}
	}
	return expr, nil
}

func (p *parser) comparison() (Expr, error) {
	left, err := p.operand()
	if err != nil {
		return nil, err
	}
	op := p.peek()
	if op.Kind != Op {
		return nil, fmt.Errorf("sqlmem: expected a comparison at %d, found %q", op.Pos, op.Text)
	}
	p.take()
	right, err := p.operand()
	if err != nil {
		return nil, err
	}
	switch {
	case left.column != "" && right.column == "":
		return columnExpr{Column: left.column, Op: op.Text, Value: right.value}, nil
	case left.column == "" && right.column == "":
		return literalExpr{Left: left.value, Op: op.Text, Right: right.value}, nil
	default:
		return nil, fmt.Errorf("sqlmem: only a column compared with a literal is supported")
	}
}

type operand struct {
	column string
	value  Value
}

func (p *parser) operand() (operand, error) {
	t := p.take()
	switch t.Kind {
	case String:
		return operand{value: Text(t.Literal)}, nil
	case Word:
		if isNumber(t.Text) {
			n, err := strconv.ParseInt(t.Text, 10, 64)
			if err != nil {
				return operand{}, fmt.Errorf("sqlmem: %q is not a number", t.Text)
			}
			return operand{value: Int(n)}, nil
		}
		return operand{column: t.Text}, nil
	default:
		return operand{}, fmt.Errorf("sqlmem: %q at %d is not a value", t.Text, t.Pos)
	}
}
