package render

import (
	"strconv"
	"strings"
	"time"

	"example.com/ledgerclose/internal/invoice"
)

// Statement renders one account's statement as an HTML page. The page is built from fragments: the
// account's display name, its id, the period label and every line's metric and reference are text
// this package escaped, so a display name registered as "<b>x</b>" is shown, not applied. The only
// markup on the page is the markup built here.
func Statement(statement invoice.Invoice) string {
	var body strings.Builder
	body.WriteString("<header>\n<h1>Statement ")
	body.WriteString(Escape(statement.AccountName))
	body.WriteString("</h1>\n<p class=\"period\">")
	body.WriteString(Escape(statement.Period.String()))
	body.WriteString(" &middot; account ")
	body.WriteString(Escape(statement.Account))
	body.WriteString(" &middot; tenant ")
	body.WriteString(Escape(statement.Tenant))
	body.WriteString("</p>\n</header>\n")

	body.WriteString("<h2>Charges</h2>\n")
	rows := make([][]string, 0, len(statement.Lines))
	for _, line := range statement.Lines {
		rows = append(rows, []string{
			line.OccurredAt.UTC().Format(time.RFC3339),
			line.Metric,
			strconv.FormatInt(line.Quantity, 10),
			line.UnitAmount.String(),
			line.Gross.String(),
			line.Fee.String(),
			line.EventID,
		})
	}
	body.WriteString(Table([]string{"instant", "metric", "quantity", "unit", "gross", "fee", "event"}, rows))

	body.WriteString("<h2>Totals</h2>\n<dl class=\"totals\">\n")
	for _, pair := range []struct{ label, value string }{
		{"Net", statement.Net.String()},
		{"Tax", statement.Tax.String()},
		{"Total", statement.Total.String()},
	} {
		body.WriteString("<dt>")
		body.WriteString(Escape(pair.label))
		body.WriteString("</dt><dd>")
		body.WriteString(Amount(pair.value, statement.Currency))
		body.WriteString("</dd>\n")
	}
	body.WriteString("</dl>\n<footer>\n<p>Issued ")
	body.WriteString(Escape(statement.Issued.UTC().Format(time.RFC3339)))
	body.WriteString("</p>\n<p>Reference ")
	body.WriteString(Escape(statement.ID))
	body.WriteString("</p>\n</footer>\n")
	return Page("Statement "+statement.Account+" "+statement.Period.String(), body.String())
}
