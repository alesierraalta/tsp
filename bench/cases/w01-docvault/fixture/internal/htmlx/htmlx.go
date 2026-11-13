// Package htmlx renders the HTML fragments the service serves. Every fragment is built here so a
// handler cannot decide on its own that a value is safe to write raw.
package htmlx

import (
	"strings"
)

// Escape renders text for an HTML text node or a quoted attribute. Every document field the
// service renders goes through it.
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

// Page wraps a rendered body in the service layout. title is escaped like any other text.
func Page(title, body string) string {
	var b strings.Builder
	b.WriteString("<!doctype html>\n<html lang=\"en\"><head><meta charset=\"utf-8\"><title>")
	b.WriteString(Escape(title))
	b.WriteString("</title></head>\n<body>\n")
	b.WriteString(body)
	b.WriteString("</body></html>\n")
	return b.String()
}

// Fragment is a cell this package already rendered: an anchor, a formatted value, an empty cell.
// Only htmlx builds one from a value, so a table can mix text and markup without a caller writing a
// tag of its own.
type Fragment string

// Table renders rows under a header row. Every cell is plain text and every cell is escaped, so a
// caller with a computed value has no way to inject markup through this helper.
func Table(headers []string, rows [][]string) string {
	fragments := make([][]Fragment, 0, len(rows))
	for _, row := range rows {
		cells := make([]Fragment, 0, len(row))
		for _, cell := range row {
			cells = append(cells, Fragment(Escape(cell)))
		}
		fragments = append(fragments, cells)
	}
	return TableFragments(headers, fragments)
}

// TableFragments renders rows whose cells this package produced. It exists so a table can carry a
// link without a second escape pass over markup that is already escaped.
func TableFragments(headers []string, rows [][]Fragment) string {
	var b strings.Builder
	b.WriteString("<table>\n<thead><tr>")
	for _, h := range headers {
		b.WriteString("<th>")
		b.WriteString(Escape(h))
		b.WriteString("</th>")
	}
	b.WriteString("</tr></thead>\n<tbody>\n")
	for _, row := range rows {
		b.WriteString("<tr>")
		for _, cell := range row {
			b.WriteString("<td>")
			b.WriteString(string(cell))
			b.WriteString("</td>")
		}
		b.WriteString("</tr>\n")
	}
	b.WriteString("</tbody>\n</table>\n")
	return b.String()
}

// Link renders an anchor. Both the target and the label are escaped, and the target must be a path
// this service serves: a target with a scheme is rendered as text instead, because an anchor is
// how a stored value would otherwise become a redirect.
func Link(href, label string) string {
	if strings.Contains(href, ":") || strings.HasPrefix(href, "//") {
		return Escape(label)
	}
	return "<a href=\"" + Escape(href) + "\">" + Escape(label) + "</a>"
}

// Attribute renders a value for a double-quoted attribute. It exists as its own name so a handler
// that needs an attribute value reaches for the escape rather than for text concatenation.
func Attribute(s string) string { return Escape(s) }
