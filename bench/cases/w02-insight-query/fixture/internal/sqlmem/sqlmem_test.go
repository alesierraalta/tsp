package sqlmem_test

import (
	"strings"
	"testing"

	"example.com/insight/internal/sqlmem"
)

func open(t *testing.T) *sqlmem.DB {
	t.Helper()
	db := sqlmem.Open()
	db.Define("events", []string{"id", "tenant", "status", "amount"})
	rows := []map[string]sqlmem.Value{
		{"id": sqlmem.Int(1), "tenant": sqlmem.Text("acme"), "status": sqlmem.Text("booked"), "amount": sqlmem.Int(100)},
		{"id": sqlmem.Int(2), "tenant": sqlmem.Text("acme"), "status": sqlmem.Text("archived"), "amount": sqlmem.Int(250)},
		{"id": sqlmem.Int(3), "tenant": sqlmem.Text("globex"), "status": sqlmem.Text("booked"), "amount": sqlmem.Int(900)},
	}
	for _, row := range rows {
		if err := db.Insert("events", row); err != nil {
			t.Fatalf("insert: %v", err)
		}
	}
	return db
}

func TestQueryBindsItsArguments(t *testing.T) {
	db := open(t)
	result, err := db.Query("SELECT id, amount FROM events WHERE tenant = ? AND status = ?", "acme", "booked")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(result.Rows) != 1 || result.Rows[0][0].String() != "1" {
		t.Fatalf("rows = %v", result.Rows)
	}
}

func TestBinderQuotesAValueSoItCannotEndTheLiteral(t *testing.T) {
	db := open(t)
	injected := "booked' OR '1'='1"
	result, err := db.Query("SELECT id FROM events WHERE status = ?", injected)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(result.Rows) != 0 {
		t.Fatalf("a bound value acted as syntax: %v", result.Rows)
	}
}

func TestQueryRefusesAnArgumentCountMismatch(t *testing.T) {
	db := open(t)
	if _, err := db.Query("SELECT id FROM events WHERE tenant = ?", "acme", "extra"); err == nil {
		t.Fatal("an argument the statement has no placeholder for was accepted")
	}
	if _, err := db.Query("SELECT id FROM events WHERE tenant = ?"); err == nil {
		t.Fatal("a placeholder without an argument was accepted")
	}
}

func TestQueryTextRunsAStatementWithNoArguments(t *testing.T) {
	db := open(t)
	result, err := db.QueryText("SELECT id FROM events WHERE status = 'booked' ORDER BY id")
	if err != nil {
		t.Fatalf("query text: %v", err)
	}
	if len(result.Rows) != 2 {
		t.Fatalf("rows = %v", result.Rows)
	}
}

func TestOrderLimitAndUnknownColumn(t *testing.T) {
	db := open(t)
	result, err := db.Query("SELECT id, amount FROM events ORDER BY amount DESC LIMIT 2")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(result.Rows) != 2 || result.Rows[0][0].String() != "3" {
		t.Fatalf("rows = %v", result.Rows)
	}
	if _, err := db.Query("SELECT nope FROM events"); err == nil {
		t.Fatal("a column the table does not carry was selected")
	}
	if _, err := db.Query("SELECT id FROM events WHERE nope = ?", "x"); err == nil {
		t.Fatal("a filter on a column the table does not carry was accepted")
	}
}

func TestParseRefusesWhatItDoesNotUnderstand(t *testing.T) {
	for _, sql := range []string{
		"DELETE FROM events",
		"SELECT id FROM events WHERE status = 'booked",
		"SELECT id FROM events WHERE status",
		"SELECT id FROM events LIMIT many",
		"SELECT id, FROM events",
	} {
		if _, err := sqlmem.Parse(sql); err == nil {
			t.Fatalf("statement %q was accepted", sql)
		}
	}
}

func TestLiteralRendersEveryValueItAccepts(t *testing.T) {
	text, err := sqlmem.Literal("o'brien")
	if err != nil || text != "'o''brien'" {
		t.Fatalf("literal = %q, %v", text, err)
	}
	if _, err := sqlmem.Literal(struct{}{}); err == nil {
		t.Fatal("a value the engine cannot store was rendered")
	}
}

func TestInsertRefusesAnUnknownColumn(t *testing.T) {
	db := open(t)
	err := db.Insert("events", map[string]sqlmem.Value{
		"id": sqlmem.Int(4), "tenant": sqlmem.Text("acme"), "status": sqlmem.Text("booked"), "amount": sqlmem.Int(1), "extra": sqlmem.Text("x"),
	})
	if err == nil || !strings.Contains(err.Error(), "extra") {
		t.Fatalf("insert = %v", err)
	}
}
