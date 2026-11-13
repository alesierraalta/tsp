package render

import (
	"strings"
)

// Table is a page table: a header row, the body rows and which columns are numeric. The table escapes
// both the headers and the cells, so a cell carrying markup is text; a numeric cell is right aligned and
// a text cell left aligned, which is the only difference the numeric flag makes.
type Table struct {
	Headers []string
	Rows    [][]string
	Numeric []bool
	Caption string
}

// HTML renders the table. A row with more cells than headers is refused by dropping the extra cells
// rather than writing them: a table whose body is wider than its head is a page an operator cannot
// read, and the values that do not fit a column have no column to sit in.
func (t Table) HTML() string {
	var b strings.Builder
	b.WriteString(`<table class="grid">`)
	if t.Caption != "" {
		b.WriteString(`<caption>`)
		b.WriteString(Escape(t.Caption))
		b.WriteString(`</caption>`)
	}
	if len(t.Headers) > 0 {
		b.WriteString(`<thead><tr>`)
		for _, header := range t.Headers {
			b.WriteString(`<th scope="col">`)
			b.WriteString(Escape(header))
			b.WriteString(`</th>`)
		}
		b.WriteString(`</tr></thead>`)
	}
	b.WriteString(`<tbody>`)
	if len(t.Rows) == 0 {
		b.WriteString(`<tr><td class="empty" colspan="`)
		b.WriteString(Integer(int64(len(t.Headers))))
		b.WriteString(`">no rows</td></tr>`)
	}
	for _, row := range t.Rows {
		b.WriteString(`<tr>`)
		for i, cell := range row {
			if i >= len(t.Headers) {
				break
			}
			b.WriteString(`<td`)
			if i < len(t.Numeric) && t.Numeric[i] {
				b.WriteString(` class="num"`)
			}
			b.WriteString(`>`)
			b.WriteString(Escape(cell))
			b.WriteString(`</td>`)
		}
		b.WriteString(`</tr>`)
	}
	b.WriteString(`</tbody></table>`)
	return b.String()
}

// DefinitionList renders label/value pairs, both escaped. An empty list renders nothing.
func DefinitionList(pairs [][2]string) string {
	if len(pairs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<dl class="facts">`)
	for _, pair := range pairs {
		b.WriteString(`<dt>`)
		b.WriteString(Escape(pair[0]))
		b.WriteString(`</dt><dd>`)
		b.WriteString(Escape(pair[1]))
		b.WriteString(`</dd>`)
	}
	b.WriteString(`</dl>`)
	return b.String()
}

// Badge renders one status as a chip. The class is derived from the status with Class, so a status
// cannot add an attribute, and the text is escaped.
func Badge(status string) string {
	return `<span class="badge ` + Class(status) + `">` + Escape(status) + `</span>`
}

// List renders an unordered list of escaped items. An empty list renders nothing.
func List(items []string) string {
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<ul class="items">`)
	for _, item := range items {
		b.WriteString(`<li>`)
		b.WriteString(Escape(item))
		b.WriteString(`</li>`)
	}
	b.WriteString(`</ul>`)
	return b.String()
}
