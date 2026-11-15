package sqlrun

import (
	"strings"
	"testing"

	"example.com/billing/internal/query"
)

// events is a table of metered events the tests read through.
func events(t *testing.T) *DB {
	t.Helper()
	db := New()
	err := db.Create("events",
		Column{Name: "id", Kind: Text},
		Column{Name: "tenant", Kind: Text},
		Column{Name: "status", Kind: Text},
		Column{Name: "quantity", Kind: Number},
		Column{Name: "day", Kind: Number},
	)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	rows := [][]Value{
		{TextValue("evt-1"), TextValue("acme"), TextValue("recorded"), NumberValue(1200), NumberValue(2)},
		{TextValue("evt-2"), TextValue("acme"), TextValue("recorded"), NumberValue(800), NumberValue(3)},
		{TextValue("evt-3"), TextValue("acme"), TextValue("billed"), NumberValue(400), NumberValue(4)},
		{TextValue("evt-4"), TextValue("globex"), TextValue("recorded"), NumberValue(300), NumberValue(2)},
	}
	for _, row := range rows {
		if err := db.Insert("events", row...); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	return db
}

func TestQueryBindsValuesAndOrders(t *testing.T) {
	db := events(t)
	b := query.Select("id", "quantity").From("events").Where("tenant", "acme").
		WhereOp("quantity", ">", 500).OrderBy("id", false)
	if err := b.Limit(10); err != nil {
		t.Fatalf("limit: %v", err)
	}
	stmt, err := b.Stmt()
	if err != nil {
		t.Fatalf("stmt: %v", err)
	}
	if strings.Contains(stmt.Text, "acme") {
		t.Fatalf("the statement carries its value: %q", stmt.Text)
	}
	result, err := db.Query(stmt)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(result.Rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(result.Rows))
	}
	if result.Rows[0][0].Text() != "evt-1" {
		t.Fatalf("first row = %v", result.Rows[0])
	}
}

func TestBoundValueIsAValueNotStatementText(t *testing.T) {
	db := events(t)
	// The same text a caller would try to inject is bound as an argument, so the engine compares it
	// with the column and matches nothing.
	b := query.Select("id").From("events").Where("status", "recorded' OR '1'='1")
	if err := b.Limit(10); err != nil {
		t.Fatalf("limit: %v", err)
	}
	stmt, err := b.Stmt()
	if err != nil {
		t.Fatalf("stmt: %v", err)
	}
	result, err := db.Query(stmt)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(result.Rows) != 0 {
		t.Fatalf("rows = %d, want none", len(result.Rows))
	}
}

func TestQueryTextParsesOperatorsInAndLike(t *testing.T) {
	db := events(t)
	result, err := db.QueryText("SELECT id FROM events WHERE tenant = ? AND status IN (?, ?) ORDER BY id LIMIT 10", "acme", "recorded", "billed")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(result.Rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(result.Rows))
	}
	result, err = db.QueryText("SELECT id, tenant FROM events WHERE tenant LIKE ? ORDER BY id LIMIT 10", "glo%")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(result.Rows) != 1 || result.Rows[0][1].Text() != "globex" {
		t.Fatalf("rows = %v", result.Strings())
	}
}

func TestDistinctAndDescendingOrder(t *testing.T) {
	db := events(t)
	stmt, err := query.Select("status").From("events").Distinct().OrderBy("status", true).Stmt()
	if err != nil {
		t.Fatalf("stmt: %v", err)
	}
	result, err := db.Query(stmt)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(result.Rows) != 2 {
		t.Fatalf("distinct statuses = %d, want 2", len(result.Rows))
	}
	if result.Rows[0][0].Text() != "recorded" {
		t.Fatalf("descending order starts at %q", result.Rows[0][0].Text())
	}
}

func TestLimitBoundsTheRows(t *testing.T) {
	db := events(t)
	result, err := db.QueryText("SELECT id FROM events ORDER BY id LIMIT 2")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(result.Rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(result.Rows))
	}
}

func TestStatementRefusals(t *testing.T) {
	db := events(t)
	cases := []string{
		"SELECT missing FROM events LIMIT 10",
		"SELECT id FROM nowhere LIMIT 10",
		"SELECT id FROM events LIMIT 0",
		"SELECT id FROM events WHERE quantity = 1.5 LIMIT 10",
		"SELECT id FROM events WHERE status = 'unterminated LIMIT 10",
		"SELECT id FROM events WHERE",
		"SELECT id FROM events LIMIT 10 trailing",
		"SELECT id FROM events WHERE status = ? LIMIT 10",
	}
	for _, text := range cases {
		if _, err := db.QueryText(text); err == nil {
			t.Errorf("%q was accepted", text)
		}
	}
}

func TestColumnsAndCount(t *testing.T) {
	db := events(t)
	if got := db.Count("events"); got != 4 {
		t.Fatalf("count = %d, want 4", got)
	}
	columns := db.Columns("events")
	if len(columns) != 5 || columns[0].Name != "id" {
		t.Fatalf("columns = %v", columns)
	}
	if err := db.Insert("events", TextValue("only-one")); err == nil {
		t.Fatal("a row with the wrong width was accepted")
	}
	if err := db.Create("events", Column{Name: "id", Kind: Text}); err == nil {
		t.Fatal("a table that carries rows was replaced")
	}
}
