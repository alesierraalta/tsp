package render

import (
	"strings"
	"testing"
	"time"

	"example.com/ledgerclose/internal/calendar"
	"example.com/ledgerclose/internal/invoice"
	"example.com/ledgerclose/internal/money"
)

func statement() invoice.Invoice {
	period, _ := calendar.Parse("2024-05")
	unit, _ := money.Parse("1.00")
	return invoice.Invoice{
		ID:          "inv-acme-team-api-2024-05",
		Tenant:      "acme",
		Account:     "team-api",
		AccountName: `Team API <b>live</b>`,
		Currency:    "USD",
		Period:      period,
		Lines: []invoice.Line{
			{
				EventID:    "evt-0001-abcd",
				Metric:     "api_call",
				Quantity:   10,
				UnitAmount: unit,
				Gross:      1000,
				Fee:        250,
				OccurredAt: time.Date(2024, 5, 10, 12, 0, 0, 0, time.UTC),
			},
		},
		Net:    250,
		Tax:    25,
		Total:  275,
		Issued: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
	}
}

func TestStatementEscapesEveryValue(t *testing.T) {
	page := Statement(statement())
	if !strings.Contains(page, "Team API &lt;b&gt;live&lt;/b&gt;") {
		t.Fatalf("the display name was not escaped:\n%s", page)
	}
	if strings.Contains(page, "<b>live</b>") {
		t.Fatal("the display name reached the page as markup")
	}
	if !strings.Contains(page, "2.75 USD") {
		t.Fatalf("the total was not rendered with its currency:\n%s", page)
	}
	if !strings.Contains(page, "10.00") {
		t.Fatalf("the line's gross was not rendered:\n%s", page)
	}
	if !strings.Contains(page, "2024-05") {
		t.Fatalf("the period was not rendered:\n%s", page)
	}
	if strings.Count(page, "<table>") != 1 {
		t.Fatalf("the page does not carry exactly one table:\n%s", page)
	}
}

func TestEscapeCoversEveryContext(t *testing.T) {
	raw := `<a href="x" title='y'>&`
	escaped := Escape(raw)
	for _, forbidden := range []string{"<", ">", `"`, "'"} {
		if strings.Contains(escaped, forbidden) {
			t.Fatalf("Escape(%q) = %q still carries %q", raw, escaped, forbidden)
		}
	}
	if !strings.Contains(escaped, "&amp;") || !strings.Contains(escaped, "&lt;") {
		t.Fatalf("Escape(%q) = %q", raw, escaped)
	}
}

func TestPageEscapesItsTitle(t *testing.T) {
	page := Page(`Report </title><script>`, "<p>body</p>")
	if strings.Contains(page, "<script>") {
		t.Fatalf("the title reached the page as markup: %s", page)
	}
	if !strings.Contains(page, "&lt;/title&gt;&lt;script&gt;") {
		t.Fatalf("the title was not escaped: %s", page)
	}
}

func TestTableEscapesHeadersAndCells(t *testing.T) {
	table := Table([]string{"<h>"}, [][]string{{"<c>", "a&b"}})
	for _, want := range []string{"&lt;h&gt;", "&lt;c&gt;", "a&amp;b"} {
		if !strings.Contains(table, want) {
			t.Fatalf("the table does not carry %q: %s", want, table)
		}
	}
	if strings.Contains(table, "<h>") || strings.Contains(table, "<c>") {
		t.Fatalf("a header or cell reached the table as markup: %s", table)
	}
}
