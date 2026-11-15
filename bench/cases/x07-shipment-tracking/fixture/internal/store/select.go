package store

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"example.com/shiptrack/internal/binder"
	"example.com/shiptrack/internal/clock"
)

// Select runs a validated selection over the store and returns its rows. The table, its columns, the
// filters and the order have already been checked against the declared schema by binder.Query, so this
// function never decides what a name means: it reads the row values the table declares and applies the
// comparison the query carries. A value is compared by its column's kind, which is why a text filter
// cannot compare against an integer column, and why two runs of the same selection return the same rows.
func (s *Store) Select(query binder.Query) ([]binder.Row, error) {
	if err := query.Validate(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.rows(query.Table)
	if err != nil {
		return nil, err
	}
	table, _ := binder.TableOf(query.Table)
	selected := make([]binder.Row, 0, len(rows))
	for _, row := range rows {
		keep, err := matches(table, row, query.Preds)
		if err != nil {
			return nil, err
		}
		if !keep {
			continue
		}
		projected := make(binder.Row, len(query.Columns))
		for _, name := range query.Columns {
			projected[name] = row[name]
		}
		selected = append(selected, projected)
	}
	orderRows(table, selected, query.Order)
	limit := query.Limit
	if limit <= 0 || limit > table.MaxRows {
		limit = table.MaxRows
	}
	if len(selected) > limit {
		selected = selected[:limit]
	}
	return selected, nil
}

// rows builds the table's rows from the records. A row is a fresh map, so a caller that keeps one
// cannot see a later write through it.
func (s *Store) rows(table string) ([]binder.Row, error) {
	switch table {
	case "events":
		rows := make([]binder.Row, 0, len(s.events))
		for _, event := range s.events {
			rows = append(rows, binder.Row{
				"id":       event.ID,
				"tenant":   event.Tenant,
				"partner":  event.Partner,
				"shipment": event.Shipment,
				"carrier":  event.Carrier,
				"lane":     event.Lane,
				"service":  event.Service,
				"kind":     string(event.Kind),
				"at":       event.Instant,
				"facility": event.Facility,
				"grams":    event.Grams,
				"recorded": event.Recorded,
			})
		}
		return rows, nil
	case "shipments":
		rows := make([]binder.Row, 0, len(s.shipments))
		for _, shipment := range s.shipments {
			rows = append(rows, binder.Row{
				"id":      shipment.ID,
				"tenant":  shipment.Tenant,
				"partner": shipment.Partner,
				"carrier": shipment.Carrier,
				"lane":    shipment.Lane,
				"service": shipment.Service,
				"grams":   shipment.Grams,
				"created": shipment.Created,
				"status":  shipment.Status,
			})
		}
		return rows, nil
	case "statements":
		rows := make([]binder.Row, 0, len(s.statements))
		for _, statement := range s.statements {
			rows = append(rows, binder.Row{
				"id":      statement.ID,
				"tenant":  statement.Tenant,
				"partner": statement.Partner,
				"period":  statement.Period.String(),
				"lines":   int64(statement.Lines),
				"net":     int64(statement.Net),
				"tax":     int64(statement.Tax),
				"total":   int64(statement.Total),
				"written": statement.Written,
			})
		}
		return rows, nil
	case "deliveries":
		rows := make([]binder.Row, 0, len(s.deliveries))
		for _, delivery := range s.deliveries {
			rows = append(rows, binder.Row{
				"id":       delivery.ID,
				"partner":  delivery.Partner,
				"endpoint": delivery.Endpoint,
				"status":   delivery.Status,
				"attempts": int64(delivery.Attempts),
				"at":       delivery.At,
			})
		}
		return rows, nil
	default:
		return nil, fmt.Errorf("%w: %q", binder.ErrUnknownTable, table)
	}
}

// matches applies the selection's filters to a row. Every predicate must hold, and a predicate that
// compares a kind with a value of another kind fails rather than ordering two unlike values.
func matches(table binder.Table, row binder.Row, preds []binder.Pred) (bool, error) {
	for _, pred := range preds {
		column, ok := table.Column(pred.Column)
		if !ok {
			return false, fmt.Errorf("%w: %q.%q", binder.ErrUnknownColumn, table.Name, pred.Column)
		}
		value, ok := row[pred.Column]
		if !ok {
			return false, fmt.Errorf("%w: %q.%q", binder.ErrUnknownColumn, table.Name, pred.Column)
		}
		ok, err := compare(column.Kind, value, pred.Op, pred.Value)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

// compare applies one comparison by kind.
func compare(kind binder.Kind, left any, op binder.Op, right any) (bool, error) {
	switch kind {
	case binder.KindText:
		a, ok := left.(string)
		b, ok2 := right.(string)
		if !ok || !ok2 {
			return false, kindMismatch(kind, left, right)
		}
		return order(strings.Compare(a, b), op), nil
	case binder.KindInt, binder.KindMoney:
		a, ok := left.(int64)
		b, ok2 := right.(int64)
		if !ok || !ok2 {
			return false, kindMismatch(kind, left, right)
		}
		return order(compareInt(a, b), op), nil
	case binder.KindTime:
		a, ok := left.(time.Time)
		b, ok2 := right.(time.Time)
		if !ok || !ok2 {
			return false, kindMismatch(kind, left, right)
		}
		return order(a.UTC().Compare(b.UTC()), op), nil
	case binder.KindBool:
		a, ok := left.(bool)
		b, ok2 := right.(bool)
		if !ok || !ok2 {
			return false, kindMismatch(kind, left, right)
		}
		return order(compareBool(a, b), op), nil
	default:
		return false, fmt.Errorf("%w: %v", binder.ErrBadValue, kind)
	}
}

func kindMismatch(kind binder.Kind, left, right any) error {
	return fmt.Errorf("%w: %v column compared %T with %T", binder.ErrBadValue, kind, left, right)
}

func compareInt(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func compareBool(a, b bool) int {
	switch {
	case a == b:
		return 0
	case !a:
		return -1
	default:
		return 1
	}
}

// order turns a three-way comparison into the operator's answer.
func order(cmp int, op binder.Op) bool {
	switch op {
	case binder.OpEq:
		return cmp == 0
	case binder.OpNe:
		return cmp != 0
	case binder.OpLt:
		return cmp < 0
	case binder.OpLe:
		return cmp <= 0
	case binder.OpGt:
		return cmp > 0
	case binder.OpGe:
		return cmp >= 0
	default:
		return false
	}
}

// orderRows sorts the rows by the selection's ordering terms. The comparison uses the same kinds the
// filters use, so a term on an instant orders by time and a term on an amount orders numerically; a
// final term on the table's declared key makes the order total, so two runs of the same selection
// return the same rows in the same order.
func orderRows(table binder.Table, rows []binder.Row, terms []binder.Order) {
	if len(rows) == 0 {
		return
	}
	ordered := append([]binder.Order(nil), terms...)
	seen := map[string]bool{}
	for _, term := range ordered {
		seen[term.Column] = true
	}
	if table.OrderKey != "" && !seen[table.OrderKey] {
		ordered = append(ordered, binder.Order{Column: table.OrderKey})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		for _, term := range ordered {
			column, ok := table.Column(term.Column)
			if !ok {
				continue
			}
			cmp := compareValues(column.Kind, rows[i][term.Column], rows[j][term.Column])
			if cmp == 0 {
				continue
			}
			if term.Desc {
				return cmp > 0
			}
			return cmp < 0
		}
		return false
	})
}

func compareValues(kind binder.Kind, left, right any) int {
	switch kind {
	case binder.KindText:
		a, _ := left.(string)
		b, _ := right.(string)
		return strings.Compare(a, b)
	case binder.KindInt, binder.KindMoney:
		a, _ := left.(int64)
		b, _ := right.(int64)
		return compareInt(a, b)
	case binder.KindTime:
		a, okA := left.(time.Time)
		b, okB := right.(time.Time)
		if !okA || !okB {
			return 0
		}
		return a.UTC().Compare(b.UTC())
	case binder.KindBool:
		a, _ := left.(bool)
		b, _ := right.(bool)
		return compareBool(a, b)
	default:
		return 0
	}
}

// ColumnNames lists the declared columns of a table, which is what a report's header prints.
func ColumnNames(table string) ([]string, error) {
	declared, ok := binder.TableOf(table)
	if !ok {
		return nil, fmt.Errorf("%w: %q", binder.ErrUnknownTable, table)
	}
	return declared.Names(), nil
}

// PeriodOf is the accounting month a row's instant falls in, for a report that groups by month.
func PeriodOf(instant time.Time) clock.Period { return clock.PeriodOf(instant) }
