package sqlrun

import (
	"fmt"
	"sort"
	"strings"

	"example.com/billing/internal/query"
)

// QueryText runs one statement over the tables and returns its answer. The statement text is parsed
// on every call, which is what makes a statement built by concatenation behave as the text it is.
func (db *DB) QueryText(text string, args ...any) (Result, error) {
	return db.Query(query.Stmt{Text: text, Args: args})
}

// Query runs a statement built by internal/query.
func (db *DB) Query(stmt query.Stmt) (Result, error) {
	parsed, err := Parse(stmt.Text, stmt.Args, db.resolve)
	if err != nil {
		return Result{}, err
	}
	db.mu.RLock()
	table, ok := db.tables[parsed.table]
	if !ok {
		db.mu.RUnlock()
		return Result{}, fmt.Errorf("sqlrun: no table %q", parsed.table)
	}
	rows := make([][]Value, len(table.Rows))
	for i, row := range table.Rows {
		rows[i] = append([]Value(nil), row...)
	}
	declared := append([]Column(nil), table.Columns...)
	db.mu.RUnlock()

	matched := make([][]Value, 0, len(rows))
	for _, row := range rows {
		if parsed.where != nil {
			ok, err := parsed.where.eval(row)
			if err != nil {
				return Result{}, fmt.Errorf("sqlrun: %s: %w", parsed.where, err)
			}
			if !ok {
				continue
			}
		}
		matched = append(matched, row)
	}
	if parsed.order != "" {
		position := columnPosition(declared, parsed.order)
		sort.SliceStable(matched, func(i, j int) bool {
			order := compare(matched[i][position], matched[j][position])
			if parsed.desc {
				return order > 0
			}
			return order < 0
		})
	}
	if parsed.limit > 0 && len(matched) > parsed.limit {
		matched = matched[:parsed.limit]
	}
	result := Result{Columns: append([]string(nil), parsed.columns...)}
	positions := make([]int, len(parsed.columns))
	for i, name := range parsed.columns {
		positions[i] = columnPosition(declared, name)
	}
	for _, row := range matched {
		projected := make([]Value, len(positions))
		for i, position := range positions {
			projected[i] = row[position]
		}
		result.Rows = append(result.Rows, projected)
	}
	if parsed.distinct {
		result.Rows = distinctRows(result.Rows)
	}
	return result, nil
}

// distinctRows removes duplicate rows, keeping the first of each.
func distinctRows(rows [][]Value) [][]Value {
	seen := make(map[string]bool, len(rows))
	out := rows[:0]
	for _, row := range rows {
		key := rowKey(row)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, row)
	}
	return out
}

// rowKey renders a row as a key, with the values separated by a byte no text value carries.
func rowKey(row []Value) string {
	var b strings.Builder
	for _, value := range row {
		b.WriteString(fmt.Sprintf("%d:%s", value.Kind(), value.Text()))
		b.WriteByte(0)
	}
	return b.String()
}

// columnPosition returns the index of a declared column.
func columnPosition(columns []Column, name string) int {
	for i, column := range columns {
		if column.Name == name {
			return i
		}
	}
	return -1
}

// resolve returns the columns of a table, for the parser.
func (db *DB) resolve(table string) ([]Column, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()
	t, ok := db.tables[table]
	if !ok {
		return nil, fmt.Errorf("sqlrun: no table %q", table)
	}
	return append([]Column(nil), t.Columns...), nil
}
