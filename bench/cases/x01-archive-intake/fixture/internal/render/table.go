package render

import (
	"example.com/archive/internal/clock"
	"fmt"
	"strings"
	"time"
)

// Cell is a value a table row carries. Only this package builds cells from tags.
type Cell string

// Table renders rows under a header row. Every cell is plain text and is escaped, so a caller with a
// computed value has no way to inject markup through this helper.
func Table(headers []string, rows [][]string) string {
	rowCells := make([][]Cell, 0, len(rows))
	for _, row := range rows {
		cells := make([]Cell, 0, len(row))
		for _, cell := range row {
			cells = append(cells, Cell(Escape(cell)))
		}
		rowCells = append(rowCells, cells)
	}
	return TableCells(headers, rowCells)
}

// TableCells renders rows whose cells a caller already rendered through this package. It exists for
// the one table that carries a link, and it is unexported-by-convention: a cell is only ever built
// from Escape, Link or Label above.
func TableCells(headers []string, rows [][]Cell) string {
	var b strings.Builder
	b.WriteString("<table>\n<thead><tr>")
	for _, header := range headers {
		b.WriteString("<th>" + Escape(header) + "</th>")
	}
	b.WriteString("</tr></thead>\n<tbody>\n")
	for _, row := range rows {
		b.WriteString("<tr>")
		for _, cell := range row {
			b.WriteString("<td>" + string(cell) + "</td>")
		}
		b.WriteString("</tr>\n")
	}
	b.WriteString("</tbody>\n</table>\n")
	return b.String()
}

// Link renders an anchor whose target and text are both escaped. A target is only ever a path this
// service serves or a provenance URI the manifest validated.
func Link(target, text string) Cell {
	return Cell("<a href=\"" + Attr(target) + "\">" + Escape(text) + "</a>")
}

// Code renders a value as preformatted text, escaped. It is the only way a page writes a <pre>.
func Code(value string) Cell { return Cell("<pre class=\"value\">" + Escape(value) + "</pre>") }

// DefinitionList renders label/value pairs, both escaped.
func DefinitionList(pairs [][2]string) string {
	var b strings.Builder
	b.WriteString("<dl>\n")
	for _, pair := range pairs {
		b.WriteString("<dt>" + Escape(pair[0]) + "</dt><dd>" + Escape(pair[1]) + "</dd>\n")
	}
	b.WriteString("</dl>\n")
	return b.String()
}

// Paragraph renders one paragraph of text.
func Paragraph(text string) string { return "<p>" + Escape(text) + "</p>\n" }

// Heading renders a heading at the given level. The level is a constant a caller writes, never a
// value from a request.
func Heading(level int, text string) string {
	if level < 1 || level > 6 {
		level = 1
	}
	tag := fmt.Sprintf("h%d", level)
	return "<" + tag + ">" + Escape(text) + "</" + tag + ">\n"
}

// Stamp renders an instant for a page in the reporting zone's abbreviation, with the UTC instant
// beside it so an operator reading a page from another zone is never guessing.
func Stamp(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return clock.Stamp(t) + " UTC"
}
