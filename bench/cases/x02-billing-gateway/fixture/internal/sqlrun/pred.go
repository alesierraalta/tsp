package sqlrun

import (
	"fmt"
	"strconv"
	"strings"
)

// statement is one parsed SELECT.
type statement struct {
	columns  []string
	distinct bool
	table    string
	where    predicate
	order    string
	desc     bool
	limit    int
}

// predicate is one node of a WHERE clause: an expression evaluated against a row.
type predicate interface {
	// eval reports whether the row satisfies the node. A row is the table's row in column order.
	eval(row []Value) (bool, error)
	// String renders the node, for the error a refused statement reports.
	String() string
}

// andPred is the conjunction of two predicates.
type andPred struct{ left, right predicate }

func (p andPred) eval(row []Value) (bool, error) {
	left, err := p.left.eval(row)
	if err != nil || !left {
		return false, err
	}
	return p.right.eval(row)
}

func (p andPred) String() string { return "(" + p.left.String() + " AND " + p.right.String() + ")" }

// orPred is the disjunction of two predicates.
type orPred struct{ left, right predicate }

func (p orPred) eval(row []Value) (bool, error) {
	left, err := p.left.eval(row)
	if err != nil {
		return false, err
	}
	if left {
		return true, nil
	}
	return p.right.eval(row)
}

func (p orPred) String() string { return "(" + p.left.String() + " OR " + p.right.String() + ")" }

// notPred is the negation of a predicate.
type notPred struct{ inner predicate }

func (p notPred) eval(row []Value) (bool, error) {
	value, err := p.inner.eval(row)
	if err != nil {
		return false, err
	}
	return !value, nil
}

func (p notPred) String() string { return "NOT " + p.inner.String() }

// operand is one side of a comparison: a column of the row or a literal.
type operand struct {
	column  int
	kind    Kind
	literal Value
	isValue bool
}

// value returns the operand's value for a row.
func (o operand) value(row []Value) (Value, error) {
	if o.isValue {
		return o.literal, nil
	}
	if o.column < 0 || o.column >= len(row) {
		return Value{}, fmt.Errorf("sqlrun: column index %d is outside the row", o.column)
	}
	return row[o.column], nil
}

// String renders the operand for an error message.
func (o operand) String() string {
	if o.isValue {
		if o.literal.Kind() == Number {
			return o.literal.Text()
		}
		return "'" + strings.ReplaceAll(o.literal.Text(), "'", "''") + "'"
	}
	return "column " + strconv.Itoa(o.column)
}

// cmpPred compares two operands.
type cmpPred struct {
	left, right operand
	op          string
}

func (p cmpPred) eval(row []Value) (bool, error) {
	left, err := p.left.value(row)
	if err != nil {
		return false, err
	}
	right, err := p.right.value(row)
	if err != nil {
		return false, err
	}
	if left.Kind() != right.Kind() {
		return false, fmt.Errorf("sqlrun: %s compares %s with %s", p, left.Kind(), right.Kind())
	}
	order := compare(left, right)
	switch p.op {
	case "=":
		return order == 0, nil
	case "!=":
		return order != 0, nil
	case "<":
		return order < 0, nil
	case "<=":
		return order <= 0, nil
	case ">":
		return order > 0, nil
	case ">=":
		return order >= 0, nil
	}
	return false, fmt.Errorf("sqlrun: %q is not a comparison", p.op)
}

func (p cmpPred) String() string {
	return p.left.String() + " " + p.op + " " + p.right.String()
}

// inPred reports whether a column's value is one of a list.
type inPred struct {
	left   operand
	values []operand
}

func (p inPred) eval(row []Value) (bool, error) {
	left, err := p.left.value(row)
	if err != nil {
		return false, err
	}
	for _, value := range p.values {
		candidate, err := value.value(row)
		if err != nil {
			return false, err
		}
		if left.Kind() != candidate.Kind() {
			continue
		}
		if compare(left, candidate) == 0 {
			return true, nil
		}
	}
	return false, nil
}

func (p inPred) String() string {
	parts := make([]string, len(p.values))
	for i, value := range p.values {
		parts[i] = value.String()
	}
	return p.left.String() + " IN (" + strings.Join(parts, ", ") + ")"
}

// likePred matches a text column against a pattern carrying % and _.
type likePred struct {
	left    operand
	pattern string
}

func (p likePred) eval(row []Value) (bool, error) {
	left, err := p.left.value(row)
	if err != nil {
		return false, err
	}
	if left.Kind() != Text {
		return false, fmt.Errorf("sqlrun: LIKE needs a text column, %s is %s", p.left, left.Kind())
	}
	return matchLike(p.pattern, left.Text()), nil
}

func (p likePred) String() string {
	return p.left.String() + " LIKE '" + strings.ReplaceAll(p.pattern, "'", "''") + "'"
}

// matchLike reports whether text matches pattern, where % is any run of characters and _ is one.
func matchLike(pattern, text string) bool {
	// The pattern is small and the text is short, so the recursive reader is the simplest correct
	// one: it walks the pattern and consumes the text as it goes.
	var walk func(p, t int) bool
	walk = func(p, t int) bool {
		for p < len(pattern) {
			switch pattern[p] {
			case '%':
				for i := t; i <= len(text); i++ {
					if walk(p+1, i) {
						return true
					}
				}
				return false
			case '_':
				if t >= len(text) {
					return false
				}
				p++
				t++
			default:
				if t >= len(text) || pattern[p] != text[t] {
					return false
				}
				p++
				t++
			}
		}
		return t == len(text)
	}
	return walk(0, 0)
}
