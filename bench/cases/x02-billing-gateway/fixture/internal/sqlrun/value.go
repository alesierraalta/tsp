// Package sqlrun is the statement engine the report projection is read through: a lexer, a parser
// for the subset of SELECT the reports use, and an evaluator over tables held in memory. A value
// reaches the evaluator either bound as an argument of a placeholder or written as a literal; a
// statement built by concatenation is parsed the same way as any other, which is why the builder in
// internal/query exists.
package sqlrun

import (
	"fmt"
	"strconv"
	"strings"
)

// Kind is the type of a value and of a column.
type Kind int

// The kinds a value can have.
const (
	// Text is a string.
	Text Kind = iota
	// Number is a signed 64-bit integer.
	Number
	// Bool is a truth value.
	Bool
)

// String renders the kind's name.
func (k Kind) String() string {
	switch k {
	case Text:
		return "text"
	case Number:
		return "number"
	case Bool:
		return "bool"
	}
	return "unknown"
}

// Value is one cell: a kind and the value of that kind. Only the field the kind names is read, so a
// zero Value is the empty text.
type Value struct {
	kind   Kind
	text   string
	number int64
	truth  bool
}

// TextValue returns a text value.
func TextValue(s string) Value { return Value{kind: Text, text: s} }

// NumberValue returns a number value.
func NumberValue(n int64) Value { return Value{kind: Number, number: n} }

// BoolValue returns a bool value.
func BoolValue(b bool) Value { return Value{kind: Bool, truth: b} }

// Kind returns the kind of the value.
func (v Value) Kind() Kind { return v.kind }

// Text returns the text of a text value, and the rendering of any other kind.
func (v Value) Text() string {
	switch v.kind {
	case Number:
		return strconv.FormatInt(v.number, 10)
	case Bool:
		if v.truth {
			return "true"
		}
		return "false"
	}
	return v.text
}

// Int returns the number of a number value, or zero for another kind.
func (v Value) Int() int64 {
	if v.kind == Number {
		return v.number
	}
	return 0
}

// Bool returns the truth of a bool value, or false for another kind.
func (v Value) Bool() bool { return v.kind == Bool && v.truth }

// String renders the value the way a response carries it.
func (v Value) String() string { return v.Text() }

// ValueOf converts a Go value into a cell: a string, an integer, a bool, or something that renders
// as text. A float is refused, because a report that computed in a float has already lost the cent
// it was measuring.
func ValueOf(value any) (Value, error) {
	switch typed := value.(type) {
	case Value:
		return typed, nil
	case string:
		return TextValue(typed), nil
	case int:
		return NumberValue(int64(typed)), nil
	case int64:
		return NumberValue(typed), nil
	case bool:
		return BoolValue(typed), nil
	case nil:
		return Value{}, fmt.Errorf("sqlrun: nil is not a value")
	case float64, float32:
		return Value{}, fmt.Errorf("sqlrun: a float is not a value the report engine carries")
	default:
		return TextValue(fmt.Sprint(value)), nil
	}
}

// compare orders two values of the same kind: -1, 0 or 1. Values of different kinds are compared as
// text, which is only reachable through a statement that compares a column with a literal of another
// kind, and the parser refuses that before evaluation.
func compare(a, b Value) int {
	if a.kind != b.kind {
		return strings.Compare(a.Text(), b.Text())
	}
	switch a.kind {
	case Number:
		switch {
		case a.number < b.number:
			return -1
		case a.number > b.number:
			return 1
		}
		return 0
	case Bool:
		switch {
		case !a.truth && b.truth:
			return -1
		case a.truth && !b.truth:
			return 1
		}
		return 0
	}
	return strings.Compare(a.text, b.text)
}

// truth reports whether a value counts as true in a predicate. Only a bool is a predicate value; a
// text or a number in a predicate position is a statement the parser refuses.
func (v Value) truthValue() bool { return v.kind == Bool && v.truth }
