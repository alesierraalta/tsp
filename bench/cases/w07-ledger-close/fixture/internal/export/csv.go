// Package export writes the ledger's CSV reports. A cell is written by encoding/csv, which quotes
// it, and every cell a caller's text reaches is passed through SafeCell first: a value a
// spreadsheet would evaluate is prefixed with an apostrophe, so the sheet shows the text instead of
// running it. Amount and quantity cells are rendered by the ledger from an integer and never come
// from a caller.
package export

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strconv"

	"example.com/ledgerclose/internal/usage"
)

// ErrWrite is returned when a report cannot be written.
var ErrWrite = fmt.Errorf("export: cannot write the report")

// MaxCellBytes bounds one exported cell: a longer value is not text a report can show, so the cell
// is truncated rather than growing the row without bound.
const MaxCellBytes = 4_000

// SafeCell makes a text cell inert in a spreadsheet. A cell whose first byte a spreadsheet
// evaluates ("=", "+", "-", "@", a tab or a carriage return) is prefixed with an apostrophe, and so
// is a cell whose first non-space byte is one of them: leading spaces do not make "=SUM(A1)" a text
// cell. The rest is written unchanged, so the sheet shows what the ledger recorded and the prefix
// marks that it did this.
func SafeCell(value string) string {
	if len(value) > MaxCellBytes {
		value = value[:MaxCellBytes]
	}
	index := 0
	for index < len(value) && (value[index] == ' ' || value[index] == '\t') {
		index++
	}
	if index == len(value) {
		return value
	}
	switch value[index] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + value
	}
	return value
}

// Usage renders the metered events: one row per event. The id, account, metric and currency are
// text a caller's ingest carried, so every one of them goes through SafeCell.
func Usage(events []usage.Event) ([]byte, error) {
	header := []string{"id", "account", "metric", "quantity", "unit_amount", "currency", "occurred_at"}
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	if err := writer.Write(header); err != nil {
		return nil, fmt.Errorf("%w: header: %v", ErrWrite, err)
	}
	for _, event := range events {
		row := []string{
			SafeCell(event.ID),
			SafeCell(event.Account),
			SafeCell(event.Metric),
			strconv.FormatInt(event.Quantity, 10),
			event.UnitAmount.String(),
			SafeCell(event.Currency),
			event.OccurredAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		}
		if err := writer.Write(row); err != nil {
			return nil, fmt.Errorf("%w: row: %v", ErrWrite, err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrWrite, err)
	}
	return buffer.Bytes(), nil
}
