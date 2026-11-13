package sqlrun

import (
	"fmt"
	"strconv"
	"strings"
)

// parser reads the token stream of one statement. It carries the bound arguments and the position
// the next placeholder takes, so a value is bound once and in order.
type parser struct {
	toks     []token
	pos      int
	args     []any
	argIndex int
	resolve  func(table string) ([]Column, error)
}

// Parse reads one statement against the tables the resolver knows.
//
// A statement is the only place a report's shape is written; the values a caller sends are arguments
// of it, and the parser refuses a value that does not have the kind of the column it is compared
// with. That refusal is what makes a concatenated filter visible: the text becomes part of the
// statement and is parsed as one.
func Parse(text string, args []any, resolve func(table string) ([]Column, error)) (*statement, error) {
	toks, err := lex(text)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks, args: args, resolve: resolve}
	return p.statement()
}

// statement reads a whole SELECT.
func (p *parser) statement() (*statement, error) {
	if err := p.keyword("select"); err != nil {
		return nil, err
	}
	out := &statement{limit: 0}
	if p.peek().kind == tokKeyword && p.peek().text == "distinct" {
		p.advance()
		out.distinct = true
	}
	columns, star, err := p.selectList()
	if err != nil {
		return nil, err
	}
	if err := p.keyword("from"); err != nil {
		return nil, err
	}
	table, err := p.ident("table")
	if err != nil {
		return nil, err
	}
	out.table = table
	declared, err := p.resolve(table)
	if err != nil {
		return nil, err
	}
	index := columnIndex(declared)
	if star {
		for _, column := range declared {
			out.columns = append(out.columns, column.Name)
		}
	} else {
		for _, name := range columns {
			if _, ok := index[name]; !ok {
				return nil, fmt.Errorf("sqlrun: table %q has no column %q", table, name)
			}
		}
		out.columns = columns
	}
	if p.atKeyword("where") {
		p.advance()
		where, err := p.orExpr(index, declared)
		if err != nil {
			return nil, err
		}
		out.where = where
	}
	if p.atKeyword("order") {
		p.advance()
		if err := p.keyword("by"); err != nil {
			return nil, err
		}
		order, err := p.ident("ordering column")
		if err != nil {
			return nil, err
		}
		if _, ok := index[order]; !ok {
			return nil, fmt.Errorf("sqlrun: table %q has no column %q to order by", table, order)
		}
		out.order = order
		if p.atKeyword("desc") {
			p.advance()
			out.desc = true
		} else if p.atKeyword("asc") {
			p.advance()
		}
	}
	if p.atKeyword("limit") {
		p.advance()
		next := p.peek()
		if next.kind != tokNumber {
			return nil, fmt.Errorf("sqlrun: LIMIT needs a number, found %s", next)
		}
		p.advance()
		limit, err := strconv.Atoi(next.text)
		if err != nil || limit < 1 {
			return nil, fmt.Errorf("sqlrun: LIMIT %q is not a positive count", next.text)
		}
		out.limit = limit
	}
	if next := p.peek(); next.kind != tokEOF {
		return nil, fmt.Errorf("sqlrun: unexpected %s after the statement", next)
	}
	if p.argIndex != len(p.args) {
		return nil, fmt.Errorf("sqlrun: the statement binds %d of %d arguments", p.argIndex, len(p.args))
	}
	return out, nil
}

// selectList reads the column list, or the star. The star is resolved once the table is known.
func (p *parser) selectList() ([]string, bool, error) {
	if p.peek().kind == tokStar {
		p.advance()
		return nil, true, nil
	}
	var columns []string
	for {
		name, err := p.ident("column")
		if err != nil {
			return nil, false, err
		}
		columns = append(columns, name)
		if p.peek().kind != tokComma {
			return columns, false, nil
		}
		p.advance()
	}
}

// orExpr reads a disjunction.
func (p *parser) orExpr(index map[string]Column, declared []Column) (predicate, error) {
	left, err := p.andExpr(index, declared)
	if err != nil {
		return nil, err
	}
	for p.atKeyword("or") {
		p.advance()
		right, err := p.andExpr(index, declared)
		if err != nil {
			return nil, err
		}
		left = orPred{left: left, right: right}
	}
	return left, nil
}

// andExpr reads a conjunction, which binds tighter than a disjunction.
func (p *parser) andExpr(index map[string]Column, declared []Column) (predicate, error) {
	left, err := p.factor(index, declared)
	if err != nil {
		return nil, err
	}
	for p.atKeyword("and") {
		p.advance()
		right, err := p.factor(index, declared)
		if err != nil {
			return nil, err
		}
		left = andPred{left: left, right: right}
	}
	return left, nil
}

// factor reads a negation, a parenthesised expression or one comparison.
func (p *parser) factor(index map[string]Column, declared []Column) (predicate, error) {
	if p.atKeyword("not") {
		p.advance()
		inner, err := p.factor(index, declared)
		if err != nil {
			return nil, err
		}
		return notPred{inner: inner}, nil
	}
	if p.peek().kind == tokLParen {
		p.advance()
		inner, err := p.orExpr(index, declared)
		if err != nil {
			return nil, err
		}
		if p.peek().kind != tokRParen {
			return nil, fmt.Errorf("sqlrun: a parenthesised expression is not closed")
		}
		p.advance()
		return inner, nil
	}
	left, err := p.operand(index, declared)
	if err != nil {
		return nil, err
	}
	if p.atKeyword("in") {
		p.advance()
		return p.inList(left, index, declared)
	}
	if p.atKeyword("like") {
		p.advance()
		pattern, err := p.operand(index, declared)
		if err != nil {
			return nil, err
		}
		if !pattern.isValue || pattern.literal.Kind() != Text {
			return nil, fmt.Errorf("sqlrun: LIKE needs a quoted pattern")
		}
		if left.kind != Text {
			return nil, fmt.Errorf("sqlrun: LIKE needs a text column")
		}
		return likePred{left: left, pattern: pattern.literal.Text()}, nil
	}
	next := p.peek()
	if next.kind != tokOperator {
		return nil, fmt.Errorf("sqlrun: %s is not followed by a comparison", next)
	}
	p.advance()
	right, err := p.operand(index, declared)
	if err != nil {
		return nil, err
	}
	if err := sameKind(left, right); err != nil {
		return nil, err
	}
	return cmpPred{left: left, right: right, op: next.text}, nil
}

// inList reads the parenthesised list of an IN test.
func (p *parser) inList(left operand, index map[string]Column, declared []Column) (predicate, error) {
	if p.peek().kind != tokLParen {
		return nil, fmt.Errorf("sqlrun: IN needs a parenthesised list")
	}
	p.advance()
	var values []operand
	for {
		value, err := p.operand(index, declared)
		if err != nil {
			return nil, err
		}
		if err := sameKind(left, value); err != nil {
			return nil, err
		}
		values = append(values, value)
		switch p.peek().kind {
		case tokComma:
			p.advance()
			continue
		case tokRParen:
			p.advance()
			if len(values) == 0 {
				return nil, fmt.Errorf("sqlrun: IN needs at least one value")
			}
			return inPred{left: left, values: values}, nil
		default:
			return nil, fmt.Errorf("sqlrun: an IN list carries %s where a value or a comma belongs", p.peek())
		}
	}
}

// operand reads a column reference, a bound argument or a literal.
func (p *parser) operand(index map[string]Column, declared []Column) (operand, error) {
	next := p.peek()
	switch next.kind {
	case tokIdent:
		p.advance()
		column, ok := index[next.text]
		if !ok {
			return operand{}, fmt.Errorf("sqlrun: no column %q in this table", next.text)
		}
		return operand{column: columnPosition(declared, column.Name), kind: column.Kind}, nil
	case tokPlaceholder:
		p.advance()
		if p.argIndex >= len(p.args) {
			return operand{}, fmt.Errorf("sqlrun: the statement asks for more arguments than it was given")
		}
		value, err := ValueOf(p.args[p.argIndex])
		if err != nil {
			return operand{}, err
		}
		p.argIndex++
		return operand{literal: value, kind: value.Kind(), isValue: true}, nil
	case tokString:
		p.advance()
		return operand{literal: TextValue(next.text), kind: Text, isValue: true}, nil
	case tokNumber:
		p.advance()
		number, err := strconv.ParseInt(next.text, 10, 64)
		if err != nil {
			return operand{}, fmt.Errorf("sqlrun: %q is not a whole number", next.text)
		}
		return operand{literal: NumberValue(number), kind: Number, isValue: true}, nil
	}
	return operand{}, fmt.Errorf("sqlrun: %s is not a value", next)
}

// sameKind refuses a comparison whose two sides have different kinds. The check happens when the
// statement is parsed, so a literal that does not fit the column it is compared with is a refusal
// rather than a row that never matches.
func sameKind(left, right operand) error {
	if left.kind == right.kind {
		return nil
	}
	return fmt.Errorf("sqlrun: %s compares %s with %s", left, left.kind, right.kind)
}

// columnIndex maps a column name to its position.
func columnIndex(columns []Column) map[string]Column {
	index := make(map[string]Column, len(columns))
	for _, column := range columns {
		index[column.Name] = column
	}
	return index
}

// peek returns the token at the current position.
func (p *parser) peek() token {
	if p.pos >= len(p.toks) {
		return token{kind: tokEOF}
	}
	return p.toks[p.pos]
}

// advance consumes the current token.
func (p *parser) advance() {
	if p.pos < len(p.toks) {
		p.pos++
	}
}

// atKeyword reports whether the current token is the keyword given.
func (p *parser) atKeyword(word string) bool {
	next := p.peek()
	return next.kind == tokKeyword && next.text == word
}

// keyword consumes the keyword given, or reports what it found instead.
func (p *parser) keyword(word string) error {
	next := p.peek()
	if next.kind != tokKeyword || next.text != word {
		return fmt.Errorf("sqlrun: expected %s, found %s", strings.ToUpper(word), next)
	}
	p.advance()
	return nil
}

// ident consumes an identifier, or reports what it found instead.
func (p *parser) ident(what string) (string, error) {
	next := p.peek()
	if next.kind != tokIdent {
		return "", fmt.Errorf("sqlrun: expected a %s, found %s", what, next)
	}
	p.advance()
	return next.text, nil
}
