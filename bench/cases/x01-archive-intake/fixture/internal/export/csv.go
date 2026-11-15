package export

import (
	"bytes"
	"encoding/csv"
	"example.com/archive/internal/store"
	"fmt"
	"strconv"
	"strings"
)

// Package export renders the archive's tabular outputs: the CSV a spreadsheet opens and the plain
// table the operator console prints. A cell is text, never a formula, and never markup.

// Headers are the columns the CSV carries, in order.
var Headers = []string{"id", "tenant", "name", "title", "kind", "pages", "bytes", "digest", "batch", "created", "committed", "tags"}

// Columns is the number of columns the header declares. A row that carries another number is refused,
// so a spreadsheet never sees a shifted line.
const Columns = 12

// CSV renders the records as a CSV document. A cell whose first byte a spreadsheet would evaluate is
// prefixed with an apostrophe, so the sheet shows the text and does not run it; the check runs on every
// cell, including the ones the archive computed itself.
func CSV(records []store.Record, limit int) ([]byte, error) {
	if limit > 0 && len(records) > limit {
		records = records[:limit]
	}
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := writer.Write(Headers); err != nil {
		return nil, fmt.Errorf("export: write header: %w", err)
	}
	for _, record := range records {
		row := Row(record)
		if len(row) != Columns {
			return nil, fmt.Errorf("export: row for %s carries %d columns, want %d", record.ID, len(row), Columns)
		}
		if err := writer.Write(row); err != nil {
			return nil, fmt.Errorf("export: write row %s: %w", record.ID, err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("export: flush: %w", err)
	}
	return buf.Bytes(), nil
}

// Row renders one record as the cells the CSV carries. Every cell goes through Cell, so the formula
// guard cannot be forgotten for one column.
func Row(record store.Record) []string {
	cells := []string{
		record.ID,
		record.Tenant,
		record.Name,
		record.Label(),
		record.Kind,
		strconv.Itoa(record.Pages),
		strconv.FormatInt(record.Bytes, 10),
		record.Digest,
		record.Batch,
		record.Created,
		record.Commit,
		strings.Join(record.Tags, " "),
	}
	for i, cell := range cells {
		cells[i] = Cell(cell)
	}
	return cells
}
