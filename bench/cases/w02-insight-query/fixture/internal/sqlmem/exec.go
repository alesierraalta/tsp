package sqlmem

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// Table is one named set of rows with a fixed column list.
type Table struct {
	Name    string
	Columns []string
	Rows    []Row
}

// DB is an in-memory database of tables. It is safe for concurrent use.
type DB struct {
	mu     sync.RWMutex
	tables map[string]*Table
}

// Open returns an empty database.
func Open() *DB { return &DB{tables: map[string]*Table{}} }

// Define declares a table. Declaring an existing table replaces it, which is what a reload does.
func (db *DB) Define(name string, columns []string) {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.tables[name] = &Table{Name: name, Columns: append([]string(nil), columns...)}
}

// Insert appends one row. A row carrying a column the table does not declare is refused, so a
// caller cannot widen a table by writing to it.
func (db *DB) Insert(table string, row map[string]Value) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	t, ok := db.tables[table]
	if !ok {
		return fmt.Errorf("sqlmem: no table %q", table)
	}
	decoded := make(Row, len(t.Columns))
	for _, col := range t.Columns {
		value, ok := row[col]
		if !ok {
			return fmt.Errorf("sqlmem: row for %q is missing column %q", table, col)
		}
		decoded[col] = value
	}
	for col := range row {
		if _, ok := decoded[col]; !ok {
			return fmt.Errorf("sqlmem: %q is not a column of %q", col, table)
		}
	}
	t.Rows = append(t.Rows, decoded)
	return nil
}

// Count reports how many rows a table carries.
func (db *DB) Count(table string) int {
	db.mu.RLock()
	defer db.mu.RUnlock()
	if t, ok := db.tables[table]; ok {
		return len(t.Rows)
	}
	return 0
}

// Result is the outcome of one statement: the selected columns and the rows, oldest first.
type Result struct {
	Columns []string
	Rows    [][]Value
}

// Query runs a statement whose arguments are bound to its placeholders. Every value the caller
// supplies travels as an argument: it is quoted into a literal here, never appended to the
// statement text by the caller, which is the rule every report follows.
func (db *DB) Query(sql string, args ...any) (Result, error) {
	if countPlaceholders(sql) != len(args) {
		return Result{}, fmt.Errorf("sqlmem: statement takes %d arguments, got %d", countPlaceholders(sql), len(args))
	}
	bound, err := bind(sql, args)
	if err != nil {
		return Result{}, err
	}
	return db.QueryText(bound)
}

// QueryText runs a statement that carries no placeholders. It exists for the statements the engine
// itself builds, where every value is a literal the caller already wrote.
func (db *DB) QueryText(sql string) (Result, error) {
	stmt, err := Parse(sql)
	if err != nil {
		return Result{}, err
	}
	db.mu.RLock()
	defer db.mu.RUnlock()
	t, ok := db.tables[stmt.Table]
	if !ok {
		return Result{}, fmt.Errorf("sqlmem: no table %q", stmt.Table)
	}

	var selected int
	for _, r := range t.Rows {
		if stmt.Where == nil {
			selected++
			continue
		}
		ok, err := stmt.Where.Match(r)
		if err != nil {
			return Result{}, err
		}
		if ok {
			selected++
		}
	}

	rows := make([]Row, 0, selected)
	for _, r := range t.Rows {
		if stmt.Where != nil {
			ok, err := stmt.Where.Match(r)
			if err != nil {
				return Result{}, err
			}
			if !ok {
				continue
			}
		}
		rows = append(rows, r)
	}
	if stmt.Order != "" {
		if _, ok := rows0Column(t, stmt.Order); !ok {
			return Result{}, fmt.Errorf("sqlmem: %q is not a column of %q", stmt.Order, t.Name)
		}
		sort.SliceStable(rows, func(i, j int) bool {
			a := rows[i][stmt.Order]
			b := rows[j][stmt.Order]
			cmp, _ := compare(a, b)
			if stmt.Desc {
				return cmp > 0
			}
			return cmp < 0
		})
	}
	if stmt.Limit > 0 && len(rows) > stmt.Limit {
		rows = rows[:stmt.Limit]
	}

	columns, err := project(t, stmt.Columns)
	if err != nil {
		return Result{}, err
	}
	result := Result{Columns: columns}
	for _, r := range rows {
		values := make([]Value, 0, len(columns))
		for _, col := range columns {
			values = append(values, r[col])
		}
		result.Rows = append(result.Rows, values)
	}
	return result, nil
}

func rows0Column(t *Table, name string) (string, bool) {
	for _, col := range t.Columns {
		if col == name {
			return col, true
		}
	}
	return "", false
}

func project(t *Table, requested []string) ([]string, error) {
	if len(requested) == 1 && requested[0] == "*" {
		return append([]string(nil), t.Columns...), nil
	}
	columns := make([]string, 0, len(requested))
	for _, col := range requested {
		if _, ok := rows0Column(t, col); !ok {
			return nil, fmt.Errorf("sqlmem: %q is not a column of %q", col, t.Name)
		}
		columns = append(columns, col)
	}
	return columns, nil
}

func countPlaceholders(sql string) int {
	n := 0
	inString := false
	for i := 0; i < len(sql); i++ {
		switch {
		case sql[i] == '\'':
			inString = !inString
		case sql[i] == '?' && !inString:
			n++
		}
	}
	return n
}

// bind quotes each argument into a literal and replaces the placeholders in order. A text argument
// is quoted with its own quotes doubled, so a value can never end the literal it sits in.
func bind(sql string, args []any) (string, error) {
	var b strings.Builder
	b.Grow(len(sql) + 16*len(args))
	next := 0
	inString := false
	for i := 0; i < len(sql); i++ {
		c := sql[i]
		if c == '\'' {
			inString = !inString
			b.WriteByte(c)
			continue
		}
		if c != '?' || inString {
			b.WriteByte(c)
			continue
		}
		if next >= len(args) {
			return "", fmt.Errorf("sqlmem: statement has more placeholders than arguments")
		}
		literal, err := Literal(args[next])
		if err != nil {
			return "", err
		}
		b.WriteString(literal)
		next++
	}
	if next != len(args) {
		return "", fmt.Errorf("sqlmem: %d arguments were not bound", len(args)-next)
	}
	return b.String(), nil
}

// Literal renders one argument as a SQL literal. Only the types the engine stores are accepted: an
// argument of any other type is refused rather than stringified into something else.
func Literal(value any) (string, error) {
	switch v := value.(type) {
	case string:
		return "'" + strings.ReplaceAll(v, "'", "''") + "'", nil
	case []byte:
		return "'" + strings.ReplaceAll(string(v), "'", "''") + "'", nil
	case int:
		return fmt.Sprintf("%d", v), nil
	case int64:
		return fmt.Sprintf("%d", v), nil
	case bool:
		if v {
			return "1", nil
		}
		return "0", nil
	default:
		return "", fmt.Errorf("sqlmem: cannot bind a %T", value)
	}
}
