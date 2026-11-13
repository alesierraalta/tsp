package query_test

import (
	"strings"
	"testing"

	"example.com/insight/internal/query"
)

func TestStatementCarriesItsValuesAsArguments(t *testing.T) {
	b := query.Select("id", "tenant").From("events").Where("tenant", "acme")
	if err := b.Limit(10); err != nil {
		t.Fatalf("limit: %v", err)
	}
	stmt, err := b.Stmt()
	if err != nil {
		t.Fatalf("stmt: %v", err)
	}
	if stmt.Text != "SELECT id, tenant FROM events WHERE tenant = ? LIMIT 10" {
		t.Fatalf("text = %q", stmt.Text)
	}
	if len(stmt.Args) != 1 || stmt.Args[0] != "acme" {
		t.Fatalf("args = %v", stmt.Args)
	}
}

func TestOperatorsAndIn(t *testing.T) {
	b := query.Select("id").From("events").WhereOp("amount", ">=", 100).WhereIn("status", []any{"booked", "pending"})
	if err := b.Limit(5); err != nil {
		t.Fatalf("limit: %v", err)
	}
	stmt, err := b.Stmt()
	if err != nil {
		t.Fatalf("stmt: %v", err)
	}
	if stmt.Text != "SELECT id FROM events WHERE amount >= ? AND status IN (?, ?) LIMIT 5" {
		t.Fatalf("text = %q", stmt.Text)
	}
	if len(stmt.Args) != 3 {
		t.Fatalf("args = %v", stmt.Args)
	}
}

func TestNamesAreValidated(t *testing.T) {
	cases := []*query.Builder{
		query.Select("id; DROP TABLE events").From("events"),
		query.Select("id").From("events; DROP TABLE events"),
		query.Select("id").From("events").OrderBy("id--", false),
	}
	for _, b := range cases {
		if err := b.Limit(5); err != nil {
			t.Fatalf("limit: %v", err)
		}
		if _, err := b.Stmt(); err == nil {
			t.Fatalf("a name outside the identifier shape was accepted: %+v", b)
		}
	}
}

func TestLimitMustBePositive(t *testing.T) {
	if err := query.Select("id").From("events").Limit(0); err == nil {
		t.Fatal("a zero limit was accepted")
	}
}

func TestOrderingDirection(t *testing.T) {
	b := query.Select("id").From("events").OrderBy("id", true)
	if err := b.Limit(1); err != nil {
		t.Fatalf("limit: %v", err)
	}
	stmt, err := b.Stmt()
	if err != nil {
		t.Fatalf("stmt: %v", err)
	}
	if !strings.Contains(stmt.Text, "ORDER BY id DESC") {
		t.Fatalf("text = %q", stmt.Text)
	}
}

func TestFilters(t *testing.T) {
	filters := []query.Filter{{Param: "status", Column: "status", Operator: "="}}
	if err := query.Known(filters, map[string]string{"status": "booked"}); err != nil {
		t.Fatalf("known: %v", err)
	}
	if err := query.Known(filters, map[string]string{"status ": "booked"}); err == nil {
		t.Fatal("an undeclared filter parameter was accepted")
	}
	b, err := query.Apply(query.Select("id").From("events"), filters, map[string]string{"status": "booked"})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if err := b.Limit(3); err != nil {
		t.Fatalf("limit: %v", err)
	}
	stmt, err := b.Stmt()
	if err != nil {
		t.Fatalf("stmt: %v", err)
	}
	if len(stmt.Args) != 1 || stmt.Args[0] != "booked" {
		t.Fatalf("args = %v", stmt.Args)
	}
}
