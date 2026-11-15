package sqlrun

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Column is one column of a table.
type Column struct {
	Name string
	Kind Kind
}

// Table is one table of the report projection.
type Table struct {
	Name    string
	Columns []Column
	Rows    [][]Value
}

// DB holds the tables the reports read. It is safe for concurrent readers and writers: a report is
// answered while the writer that maintains the projection is still running.
type DB struct {
	mu     sync.RWMutex
	tables map[string]*Table
}

// New returns an empty database.
func New() *DB {
	return &DB{tables: make(map[string]*Table)}
}

// Create declares a table. A table that already exists is replaced only when it is empty, so a
// second Create is a programming error rather than a silent loss of rows.
func (db *DB) Create(name string, columns ...Column) error {
	if !ValidName(name) {
		return fmt.Errorf("sqlrun: %q is not a table name", name)
	}
	seen := make(map[string]bool, len(columns))
	for _, column := range columns {
		if !ValidName(column.Name) {
			return fmt.Errorf("sqlrun: %q is not a column name", column.Name)
		}
		if seen[column.Name] {
			return fmt.Errorf("sqlrun: table %q declares %q twice", name, column.Name)
		}
		seen[column.Name] = true
	}
	if len(columns) == 0 {
		return fmt.Errorf("sqlrun: table %q has no columns", name)
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	if existing, ok := db.tables[name]; ok && len(existing.Rows) > 0 {
		return fmt.Errorf("sqlrun: table %q already carries rows", name)
	}
	db.tables[name] = &Table{Name: name, Columns: append([]Column(nil), columns...)}
	return nil
}

// Insert appends one row. The row must carry one value per column, each of the column's kind: a text
// value in a number column is refused at write time rather than answered wrongly at read time.
func (db *DB) Insert(table string, values ...Value) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	t, ok := db.tables[table]
	if !ok {
		return fmt.Errorf("sqlrun: no table %q", table)
	}
	if len(values) != len(t.Columns) {
		return fmt.Errorf("sqlrun: table %q has %d columns, row carries %d", table, len(t.Columns), len(values))
	}
	for i, value := range values {
		if value.Kind() != t.Columns[i].Kind {
			return fmt.Errorf("sqlrun: column %q.%s is %s, row carries %s",
				table, t.Columns[i].Name, t.Columns[i].Kind, value.Kind())
		}
	}
	t.Rows = append(t.Rows, append([]Value(nil), values...))
	return nil
}

// Replace empties a table and inserts the rows given, which is how the projection is rebuilt when a
// record changes. The swap happens under one lock, so a reader sees the old table or the new one and
// never half of each.
func (db *DB) Replace(table string, rows [][]Value) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	t, ok := db.tables[table]
	if !ok {
		return fmt.Errorf("sqlrun: no table %q", table)
	}
	for _, row := range rows {
		if len(row) != len(t.Columns) {
			return fmt.Errorf("sqlrun: table %q has %d columns, row carries %d", table, len(t.Columns), len(row))
		}
		for i, value := range row {
			if value.Kind() != t.Columns[i].Kind {
				return fmt.Errorf("sqlrun: column %q.%s is %s, row carries %s",
					table, t.Columns[i].Name, t.Columns[i].Kind, value.Kind())
			}
		}
	}
	t.Rows = make([][]Value, len(rows))
	for i, row := range rows {
		t.Rows[i] = append([]Value(nil), row...)
	}
	return nil
}

// Count returns the number of rows in a table.
func (db *DB) Count(table string) int {
	db.mu.RLock()
	defer db.mu.RUnlock()
	if t, ok := db.tables[table]; ok {
		return len(t.Rows)
	}
	return 0
}

// Tables lists the table names in order.
func (db *DB) Tables() []string {
	db.mu.RLock()
	defer db.mu.RUnlock()
	names := make([]string, 0, len(db.tables))
	for name := range db.tables {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Columns returns the declared columns of a table.
func (db *DB) Columns(table string) []Column {
	db.mu.RLock()
	defer db.mu.RUnlock()
	t, ok := db.tables[table]
	if !ok {
		return nil
	}
	return append([]Column(nil), t.Columns...)
}

// Result is the answer to a statement: the column names in select order and the rows.
type Result struct {
	Columns []string
	Rows    [][]Value
}

// Len returns the number of rows answered.
func (r Result) Len() int { return len(r.Rows) }

// Strings renders the rows as text, which is what a CSV export and a test both want.
func (r Result) Strings() [][]string {
	out := make([][]string, len(r.Rows))
	for i, row := range r.Rows {
		cells := make([]string, len(row))
		for j, value := range row {
			cells[j] = value.Text()
		}
		out[i] = cells
	}
	return out
}

// Column returns the index of a column of the result, or -1 when the answer does not carry it.
func (r Result) Column(name string) int {
	for i, column := range r.Columns {
		if column == name {
			return i
		}
	}
	return -1
}

// ValidName reports whether value is a table or column name.
func ValidName(value string) bool {
	if value == "" || len(value) > 63 {
		return false
	}
	if !isIdentStart(value[0]) {
		return false
	}
	for i := 0; i < len(value); i++ {
		if !isIdentPart(value[i]) {
			return false
		}
	}
	return !keywords[strings.ToLower(value)]
}
