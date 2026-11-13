package binder

import (
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"example.com/shiptrack/internal/clock"
	"example.com/shiptrack/internal/money"
)

// ErrTooManyValues is reported when a request carries a parameter twice. Two values are not one filter
// with two meanings: a caller that sends `carrier=a&carrier=b` is refused, rather than handed whichever
// of the two the parser happened to read last.
var ErrTooManyValues = errors.New("binder: parameter repeated")

// maxTextValue bounds a text filter value. A filter is a comparison, not a document: a value larger than
// this is refused rather than carried into the engine.
const maxTextValue = 200

// Defaults are the request parameters the binder reads itself. They are reserved names: a definition
// cannot declare a filter called "limit" and quietly shadow the page bound.
var Defaults = []string{"limit", "order"}

// Filter returns the declared filter of a request parameter name.
func (d Declaration) Filter(name string) (Filter, bool) {
	for _, filter := range d.Filters {
		if filter.Name == name {
			return filter, true
		}
	}
	return Filter{}, false
}

// Query fills the declaration in from a request. Every parameter is checked: an undeclared one is
// refused rather than ignored, an order name is looked up in the declaration's table so the column that
// reaches the selection is the declared one, and a limit above the declaration's cap is refused instead
// of clamped, so a caller cannot read a page that is not the page it asked for. The result is validated
// by Query.Validate, which is what makes a name from a request either a declared name or an error.
func (d Declaration) Query(values url.Values) (Query, error) {
	table, ok := TableOf(d.Table)
	if !ok {
		return Query{}, fmt.Errorf("%w: %q", ErrUnknownTable, d.Table)
	}
	limit := d.PageSize
	var order []Order
	var filters []Pred
	for _, key := range sortedKeys(values) {
		raw := values[key]
		if len(raw) != 1 {
			return Query{}, fmt.Errorf("%w: %q carries %d values", ErrTooManyValues, key, len(raw))
		}
		value := raw[0]
		switch key {
		case "limit":
			parsed, err := strconv.Atoi(value)
			if err != nil || parsed < 1 {
				return Query{}, fmt.Errorf("%w: limit %q", ErrLimit, value)
			}
			if parsed > d.MaxRows {
				return Query{}, fmt.Errorf("%w: limit %d is above the definition's %d", ErrLimit, parsed, d.MaxRows)
			}
			limit = parsed
			continue
		case "order":
			declared, ok := d.Orders[value]
			if !ok {
				return Query{}, fmt.Errorf("%w: %q", ErrUnknownOrder, value)
			}
			if _, ok := table.Column(declared.Column); !ok {
				return Query{}, fmt.Errorf("%w: %q.%q", ErrUnknownColumn, d.Table, declared.Column)
			}
			order = []Order{declared}
			continue
		}
		filter, ok := d.Filter(key)
		if !ok {
			return Query{}, fmt.Errorf("%w: %q", ErrUnknownFilter, key)
		}
		if _, ok := table.Column(filter.Column); !ok {
			return Query{}, fmt.Errorf("%w: %q.%q", ErrUnknownColumn, d.Table, filter.Column)
		}
		parsed, err := parseValue(filter, value)
		if err != nil {
			return Query{}, err
		}
		op := filter.Op
		if op == "" {
			op = OpEq
		}
		if _, err := op.SQL(); err != nil {
			return Query{}, err
		}
		filters = append(filters, Pred{Column: filter.Column, Op: op, Value: parsed})
	}
	if len(order) == 0 {
		order = append([]Order(nil), d.DefaultOrder...)
	}
	query := Query{
		Table:   d.Table,
		Columns: append([]string(nil), d.Columns...),
		Preds:   filters,
		Order:   order,
		Limit:   limit,
	}
	if err := query.Validate(); err != nil {
		return Query{}, err
	}
	return query, nil
}

// parseValue reads a filter value by its column's kind. A value that does not parse is refused with the
// kind it failed: a caller that sends a date where an amount belongs is told which one it sent.
func parseValue(filter Filter, raw string) (any, error) {
	if raw == "" {
		return nil, fmt.Errorf("%w: %q is empty", ErrBadValue, filter.Name)
	}
	if strings.TrimSpace(raw) != raw {
		return nil, fmt.Errorf("%w: %q carries surrounding space", ErrBadValue, filter.Name)
	}
	switch filter.Kind {
	case KindText:
		if len(raw) > maxTextValue {
			return nil, fmt.Errorf("%w: %q is %d bytes", ErrBadValue, filter.Name, len(raw))
		}
		if err := checkTextValue(raw); err != nil {
			return nil, err
		}
		return raw, nil
	case KindInt:
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%w: %q wants an integer", ErrBadValue, filter.Name)
		}
		return parsed, nil
	case KindMoney:
		amount, err := money.Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("%w: %q wants an amount", ErrBadValue, filter.Name)
		}
		return int64(amount), nil
	case KindTime:
		instant, err := clock.ParseInstant(raw)
		if err != nil {
			return nil, fmt.Errorf("%w: %q wants an RFC 3339 instant", ErrBadValue, filter.Name)
		}
		return instant, nil
	case KindBool:
		switch raw {
		case "true":
			return true, nil
		case "false":
			return false, nil
		default:
			return nil, fmt.Errorf("%w: %q wants true or false", ErrBadValue, filter.Name)
		}
	default:
		return nil, fmt.Errorf("%w: %q has no kind", ErrBadValue, filter.Name)
	}
}

// checkTextValue refuses a control character in a text filter. The value is bound rather than
// concatenated, so this is not an injection guard: it is what keeps a parameter out of a log line and out
// of a page that echoes the selection it ran.
func checkTextValue(value string) error {
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("%w: value carries a control character", ErrBadValue)
		}
	}
	return nil
}

// sortedKeys lists a request's parameter names in a stable order, so two runs of the same request select
// the same rows and a refusal names the first problem in the same parameter.
func sortedKeys(values url.Values) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// ParseTime is the instant parser a definition's time filter uses, exported so a report can describe the
// form it accepts in its own help text.
func ParseTime(raw string) (time.Time, error) { return clock.ParseInstant(raw) }
