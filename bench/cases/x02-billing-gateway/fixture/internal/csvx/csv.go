// Package csvx renders a table as a CSV file. It is the only place in the billing service that writes
// a comma-separated row, so a quote, a comma or a line break inside a value is handled once.
package csvx

import (
	"strings"
)

// Render returns the CSV text of a header and its rows. The header is written as it is given and
// every cell is quoted when it has to be, which is what a spreadsheet and a parser both expect.
func Render(header []string, rows [][]string) string {
	var b strings.Builder
	writeRow(&b, header)
	for _, row := range rows {
		writeRow(&b, row)
	}
	return b.String()
}

// RenderRows returns the CSV text of rows without a header, for a file a caller builds itself.
func RenderRows(rows [][]string) string {
	var b strings.Builder
	for _, row := range rows {
		writeRow(&b, row)
	}
	return b.String()
}

// writeRow writes one row and its line break.
func writeRow(b *strings.Builder, row []string) {
	for i, cell := range row {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(Cell(cell))
	}
	b.WriteByte('\n')
}

// Cell renders one cell. A cell carrying a comma, a quote, a line break or a leading space is quoted,
// and a quote inside it is doubled, so a reader that splits on commas recovers the value it was given.
func Cell(value string) string {
	if value == "" {
		return ""
	}
	needs := strings.ContainsAny(value, ",\"\n\r") || strings.HasPrefix(value, " ") || strings.HasSuffix(value, " ")
	if !needs {
		return value
	}
	return `"` + strings.ReplaceAll(value, `"`, `""`) + `"`
}

// Rows converts a header and a list of string slices into the row list Render takes, which is how a
// caller that has typed records hands them over without rendering each cell itself.
func Rows(header []string, records [][]string) [][]string {
	rows := make([][]string, 0, len(records)+1)
	rows = append(rows, header)
	rows = append(rows, records...)
	return rows
}

// Column returns the values of one column of a row list, or nil when the column does not exist.
func Column(rows [][]string, index int) []string {
	if index < 0 {
		return nil
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		if index >= len(row) {
			return nil
		}
		out = append(out, row[index])
	}
	return out
}

// Width returns the widest row of a set, which a fixed-width table renderer needs.
func Width(rows [][]string) int {
	width := 0
	for _, row := range rows {
		if len(row) > width {
			width = len(row)
		}
	}
	return width
}
