package render_test

import (
	"strings"
	"testing"

	"example.com/shiptrack/internal/clock"
	"example.com/shiptrack/internal/identity"
	"example.com/shiptrack/internal/money"
	"example.com/shiptrack/internal/render"
	"example.com/shiptrack/internal/statement"
)

func TestEscapeAndAttr(t *testing.T) {
	if got := render.Escape(`<b>a & b</b>`); got != "&lt;b&gt;a &amp; b&lt;/b&gt;" {
		t.Fatalf("Escape = %q", got)
	}
	if got := render.Attr(`x" onmouseover="y`); strings.Contains(got, `" onmouseover="`) {
		t.Fatalf("Attr left the attribute open: %q", got)
	}
}

func TestTableEscapesHeadersAndCells(t *testing.T) {
	table := render.Table{
		Headers: []string{"Shipment", "<script>"},
		Rows:    [][]string{{"shp_1", "<img src=x onerror=alert(1)>"}},
		Numeric: []bool{false, false},
	}
	page := table.HTML()
	if strings.Contains(page, "<script>") || strings.Contains(page, "<img") {
		t.Fatalf("the table wrote markup it was handed: %s", page)
	}
	if !strings.Contains(page, "&lt;script&gt;") {
		t.Fatalf("the header was not escaped: %s", page)
	}
}

func TestStatementPageEscapesThePartnerName(t *testing.T) {
	partner := identity.Partner{Tenant: "acme", ID: "shop", Name: "<b>Shop</b>"}
	built, err := statement.Build(partner, clock.Period{Year: 2024, Month: 5}, []statement.Item{{
		Shipment: "shp_00000000000000000000000000000000",
		Carrier:  "fastco",
		Service:  "ground",
		Lane:     "mad>bcn",
		Grams:    1500,
		Base:     money.Amount(698),
		Fuel:     money.Amount(59),
	}}, 1900, clock.Period{Year: 2024, Month: 5}.Start())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	page, err := render.StatementPage(built)
	if err != nil {
		t.Fatalf("page: %v", err)
	}
	if strings.Contains(page, "<b>") || strings.Contains(page, "</b>") {
		t.Fatalf("the partner name reached the page as markup: %s", page)
	}
	if !strings.Contains(page, "&lt;b&gt;Shop&lt;/b&gt;") {
		t.Fatalf("the partner name is not shown as text")
	}
	if !strings.Contains(page, "2024-05") {
		t.Fatalf("the page does not name its period")
	}
}

func TestNoteKeepsLineBreaksAsText(t *testing.T) {
	got := render.Note("first\nsecond <b>")
	if strings.Contains(got, "<b>") {
		t.Fatalf("Note wrote markup: %q", got)
	}
	if !strings.Contains(got, "<br>") {
		t.Fatalf("Note did not keep the line break: %q", got)
	}
}
