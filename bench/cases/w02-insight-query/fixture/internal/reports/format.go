package reports

import (
	"encoding/csv"
	"strings"

	"example.com/insight/internal/sqlmem"
)

// CSV renders a result as a CSV file: a header row, then one row per record. Every value is quoted
// by the encoder, so a value carrying a comma or a quote cannot split or extend a record.
func CSV(result sqlmem.Result) (string, error) {
	var b strings.Builder
	w := csv.NewWriter(&b)
	if err := w.Write(result.Columns); err != nil {
		return "", err
	}
	for _, values := range result.Rows {
		record := make([]string, 0, len(values))
		for _, value := range values {
			record = append(record, value.String())
		}
		if err := w.Write(record); err != nil {
			return "", err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return "", err
	}
	return b.String(), nil
}

// Total sums one integer column of a result. It reports an error for a column the result does not
// carry, and for a cell that is not a number: a total that silently skipped a column would read as
// a smaller total.
func Total(result sqlmem.Result, column string) (int64, error) {
	at := -1
	for i, col := range result.Columns {
		if col == column {
			at = i
			break
		}
	}
	if at < 0 {
		return 0, errNoColumn(column)
	}
	var total int64
	for _, values := range result.Rows {
		if at >= len(values) || !values[at].IsInt {
			return 0, errNotANumber(column)
		}
		total += values[at].Int
	}
	return total, nil
}

type columnError string

func (e columnError) Error() string { return "reports: " + string(e) }

func errNoColumn(column string) error { return columnError("no column " + column) }

func errNotANumber(column string) error { return columnError("column " + column + " is not a number") }
