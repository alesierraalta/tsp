// Package render builds the HTML the service serves. Every fragment is built here so a handler cannot
// decide on its own that a value is safe to write raw.
package render

import (
	"strings"
)

// Escape renders text for an HTML text node or a quoted attribute. Every value a statement carries goes
// through it: a display name, a metric, a reference, an amount.
func Escape(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&#34;")
		case '\'':
			b.WriteString("&#39;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Page wraps a rendered body in the service layout. The title is escaped like any other text.
func Page(title, body string) string {
	return "<!doctype html>\n<html lang=\"en\"><head><meta charset=\"utf-8\"><title>" +
		Escape(title) + "</title></head>\n<body>\n" + body + "</body></html>\n"
}

// Table renders rows under a header row. A header and a cell are plain text and both are escaped here, so
// a caller with a computed value has no way to inject markup through this helper.
func Table(headers []string, rows [][]string) string {
	var b strings.Builder
	b.WriteString("<table>\n<thead><tr>")
	for _, header := range headers {
		b.WriteString("<th>")
		b.WriteString(Escape(header))
		b.WriteString("</th>")
	}
	b.WriteString("</tr></thead>\n<tbody>\n")
	for _, row := range rows {
		b.WriteString("<tr>")
		for _, cell := range row {
			b.WriteString("<td>")
			b.WriteString(Escape(cell))
			b.WriteString("</td>")
		}
		b.WriteString("</tr>\n")
	}
	b.WriteString("</tbody>\n</table>\n")
	return b.String()
}

// Amount renders an amount for a page: the number in the statement's currency, escaped like any other text
// so the reader never has to trust the formatter.
func Amount(value, currency string) string {
	return Escape(value + " " + currency)
}
