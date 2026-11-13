package export

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"example.com/archive/internal/store"
	"example.com/archive/internal/textutil"
)

// CellWidth is the largest number of runes one text-table cell prints. A longer value is shortened at a
// rune boundary, so the console's columns stay aligned whatever the title carries.
const CellWidth = 28

// TextTable renders records as the fixed-width table the operator console prints: a header line, a
// separator and one line per record. Every cell is shortened through textutil.Label, which counts
// runes rather than bytes.
func TextTable(records []store.Record) string {
	headers := []string{"name", "kind", "tenant", "pages", "bytes", "committed"}
	rows := make([][]string, 0, len(records))
	for _, record := range records {
		rows = append(rows, []string{
			truncate(record.Name),
			truncate(record.Kind),
			truncate(record.Tenant),
			fmt.Sprintf("%d", record.Pages),
			fmt.Sprintf("%d", record.Bytes),
			truncate(record.Commit),
		})
	}
	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = utf8.RuneCountInString(header)
	}
	for _, row := range rows {
		for i, cell := range row {
			if width := utf8.RuneCountInString(cell); width > widths[i] {
				widths[i] = width
			}
		}
	}
	var b strings.Builder
	writeRow(&b, headers, widths)
	writeSeparator(&b, widths)
	for _, row := range rows {
		writeRow(&b, row, widths)
	}
	return b.String()
}

// writeRow writes one line with the cells padded to the column widths.
func writeRow(b *strings.Builder, cells []string, widths []int) {
	for i, cell := range cells {
		b.WriteString(cell)
		if i == len(cells)-1 {
			break
		}
		pad := widths[i] - utf8.RuneCountInString(cell) + 2
		b.WriteString(strings.Repeat(" ", pad))
	}
	b.WriteString("\n")
}

// writeSeparator writes the dashed line under a header row.
func writeSeparator(b *strings.Builder, widths []int) {
	for i, width := range widths {
		b.WriteString(strings.Repeat("-", width))
		if i == len(widths)-1 {
			break
		}
		b.WriteString("  ")
	}
	b.WriteString("\n")
}

// truncate shortens one cell to the table's cell width.
func truncate(value string) string {
	if utf8.RuneCountInString(value) <= CellWidth {
		return value
	}
	return textutil.Label(value)
}

// Rows renders the count line the console prints above a table.
func Rows(count int) string {
	if count == 1 {
		return "1 object"
	}
	return fmt.Sprintf("%d objects", count)
}
