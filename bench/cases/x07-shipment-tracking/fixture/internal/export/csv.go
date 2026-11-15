// Package export writes a report as CSV and as a fixed-width table. A text cell whose first non-space
// byte a spreadsheet would evaluate is prefixed with an apostrophe, so a scan note that begins with
// "=" is text in the sheet and not a formula; an amount cell is rendered from integer minor units by
// the report and may begin with a minus sign, because a negative amount is a number the sheet should
// read as one.
package export

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"
)

// maxCell bounds one cell. A note longer than this is refused rather than exported: a cell is a value,
// not a document, and a spreadsheet column of unbounded text is not a report a partner can use.
const maxCell = 4096

// GuardCell applies the spreadsheet rule to one cell. A numeric cell is returned unchanged; a text cell
// whose first non-space byte is "=", "+", "-", "@", a tab or a carriage return is prefixed with an
// apostrophe, which every spreadsheet reads as "this is text".
func GuardCell(value string, numeric bool) string {
	if numeric {
		return value
	}
	rest := strings.TrimLeft(value, " ")
	if rest == "" {
		return value
	}
	switch rest[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + value
	}
	return value
}

// CSV writes a report as CSV. The header carries the report name and the selection it ran, so a file an
// operator downloads says what it is; the selection is rendered with `?` in place of every value, so
// the header cannot carry a value into the file.
func CSV(w io.Writer, name, selection string, headers []string, rows [][]string, numeric []bool) error {
	if w == nil {
		return fmt.Errorf("export: no writer")
	}
	writer := csv.NewWriter(w)
	for _, cell := range append(append([]string(nil), headers...), selection) {
		if len(cell) > maxCell {
			return fmt.Errorf("export: header cell is %d bytes", len(cell))
		}
	}
	if err := writer.Write([]string{"# report", name}); err != nil {
		return fmt.Errorf("export: header: %w", err)
	}
	if err := writer.Write([]string{"# selection", selection}); err != nil {
		return fmt.Errorf("export: header: %w", err)
	}
	if err := writer.Write(guardCells(headers, nil)); err != nil {
		return fmt.Errorf("export: header: %w", err)
	}
	for i, row := range rows {
		if len(row) != len(headers) {
			return fmt.Errorf("export: row %d has %d cells, want %d", i, len(row), len(headers))
		}
		if err := writer.Write(guardCells(row, numeric)); err != nil {
			return fmt.Errorf("export: row %d: %w", i, err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return fmt.Errorf("export: flush: %w", err)
	}
	return nil
}

// guardCells applies GuardCell to a row of cells.
func guardCells(row []string, numeric []bool) []string {
	cells := make([]string, 0, len(row))
	for i, cell := range row {
		isNumeric := i < len(numeric) && numeric[i]
		cells = append(cells, GuardCell(cell, isNumeric))
	}
	return cells
}

// Table writes a fixed-width table: a header row, a rule, the rows, and a rule. A cell wider than its
// column is clipped with an ellipsis rather than pushing the table out of shape.
func Table(w io.Writer, headers []string, rows [][]string) error {
	if w == nil {
		return fmt.Errorf("export: no writer")
	}
	widths := make([]int, len(headers))
	for i, header := range headers {
		widths[i] = len(header)
	}
	for _, row := range rows {
		for i, cell := range row {
			if i >= len(widths) {
				break
			}
			if len(cell) > widths[i] {
				widths[i] = len(cell)
			}
		}
	}
	// A column is capped at 40 bytes so a long note cannot make the table wider than a terminal.
	for i := range widths {
		if widths[i] > 40 {
			widths[i] = 40
		}
	}
	if _, err := fmt.Fprintln(w, line(headers, widths)); err != nil {
		return fmt.Errorf("export: header: %w", err)
	}
	if _, err := fmt.Fprintln(w, rule(widths)); err != nil {
		return fmt.Errorf("export: rule: %w", err)
	}
	for _, row := range rows {
		if _, err := fmt.Fprintln(w, line(row, widths)); err != nil {
			return fmt.Errorf("export: row: %w", err)
		}
	}
	if _, err := fmt.Fprintln(w, rule(widths)); err != nil {
		return fmt.Errorf("export: rule: %w", err)
	}
	return nil
}

func line(cells []string, widths []int) string {
	parts := make([]string, 0, len(widths))
	for i, width := range widths {
		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}
		if len(cell) > width {
			if width > 1 {
				cell = cell[:width-1] + "…"
			} else {
				cell = cell[:width]
			}
		}
		parts = append(parts, cell+strings.Repeat(" ", width-len(cell)))
	}
	return strings.TrimRight(strings.Join(parts, "  "), " ")
}

func rule(widths []int) string {
	total := 0
	for _, width := range widths {
		total += width
	}
	total += 2 * (len(widths) - 1)
	return strings.Repeat("-", total)
}
