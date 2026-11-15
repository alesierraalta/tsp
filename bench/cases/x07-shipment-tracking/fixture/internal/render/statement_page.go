package render

import (
	"strings"

	"example.com/shiptrack/internal/clock"
	"example.com/shiptrack/internal/money"
	"example.com/shiptrack/internal/statement"
	"example.com/shiptrack/internal/textutil"
)

// StatementPage renders a partner's statement. Every value on the page comes from the statement — the
// partner's display name, the lane, the carrier, the service — and each of them is escaped by the
// fragment helpers, so a partner registered with markup in its name shows the markup as text rather than
// as an element the browser would run.
func StatementPage(s statement.Statement) (string, error) {
	if !s.Period.Valid() {
		return "", statement.ErrInvalid
	}
	facts := DefinitionList([][2]string{
		{"Partner", s.Partner.ID},
		{"Tenant", s.Tenant},
		{"Period", s.Period.String()},
		{"Lines", Integer(int64(len(s.Lines)))},
		{"Currency", s.Currency},
		{"Built", clock.FormatInstant(s.Built)},
	})
	rows := make([][]string, 0, len(s.Lines))
	numeric := []bool{false, false, true, true, true, true, true}
	for _, line := range s.Lines {
		rows = append(rows, []string{
			line.At.UTC().Format("2006-01-02"),
			line.Shipment,
			line.Carrier + " " + line.Service,
			textutil.Grams(line.Grams),
			line.Base.String(),
			line.Fuel.String(),
			line.Tax.String(),
			line.Total.String(),
		})
	}
	// The table's header order matches the numeric flags above; a column added to one without the other
	// would shift every amount one cell to the right, so the two live side by side.
	lines := Table{
		Caption: "Shipment lines",
		Headers: []string{"Date", "Shipment", "Carrier", "Weight kg", "Freight", "Fuel", "Tax", "Total"},
		Rows:    rows,
		Numeric: append([]bool{false, false, false}, numeric...),
	}
	totals := Table{
		Caption: "Totals",
		Headers: []string{"Freight", "Fuel", "Net", "Tax", "Total"},
		Rows: [][]string{{
			s.Base.String(),
			s.Fuel.String(),
			s.Net.String(),
			s.Tax.String(),
			s.Total.String(),
		}},
		Numeric: []bool{true, true, true, true, true},
	}
	var body strings.Builder
	body.WriteString(Breadcrumb([]string{"partners", s.Partner.ID, "statements", s.Period.String()}))
	body.WriteString(Section("Statement", facts+lines.HTML()+totals.HTML()))
	if s.Partner.Name != "" {
		body.WriteString(Section("Billed to", `<p class="partner">`+Escape(s.Partner.Name)+`</p>`))
	}
	body.WriteString(Section("Notes", List([]string{
		"Weight is billed in whole kilograms, with part of a kilogram billed as a whole one.",
		"Fuel and tax are allocated across the lines in proportion to their weight and never exceed a line's freight.",
		"Amounts are integer minor units until they are rendered here.",
	})))
	return Page("Statement "+s.Partner.ID, s.Period.String(), body.String()), nil
}

// StatementIndex renders the list of statements a partner has closed, newest first.
func StatementIndex(partner string, statements []statement.Statement) string {
	rows := make([][]string, 0, len(statements))
	for _, s := range statements {
		rows = append(rows, []string{
			s.Period.String(),
			Integer(int64(len(s.Lines))),
			s.Net.String(),
			s.Tax.String(),
			s.Total.String(),
		})
	}
	table := Table{
		Caption: "Closed statements",
		Headers: []string{"Period", "Lines", "Net", "Tax", "Total"},
		Rows:    rows,
		Numeric: []bool{false, true, true, true, true},
	}
	body := Breadcrumb([]string{"partners", partner, "statements"}) + table.HTML()
	return Page("Statements", partner, body)
}

// Amount renders an amount as the page writes it. It is here so a page that has an amount and a page
// that has a line render it the same way.
func Amount(value money.Amount) string { return Escape(value.String()) }
