package audit

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strconv"
)

// CSVHeaders are the columns the audit export carries, in order.
var CSVHeaders = []string{"id", "at", "subject", "tenant", "action", "target", "outcome", "detail"}

// CSV renders trail entries as the CSV an operator keeps with an incident. A cell a spreadsheet would
// evaluate is prefixed, exactly like the object export: an audit detail is text, never a formula.
func CSV(entries []Entry, limit int) ([]byte, error) {
	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}
	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	if err := writer.Write(CSVHeaders); err != nil {
		return nil, fmt.Errorf("audit: write header: %w", err)
	}
	for _, entry := range entries {
		row := []string{
			entry.ID,
			entry.At,
			entry.Subject,
			entry.Tenant,
			entry.Action,
			entry.Target,
			entry.Outcome,
			entry.Detail,
		}
		for i, cell := range row {
			row[i] = formulaSafe(cell)
		}
		if err := writer.Write(row); err != nil {
			return nil, fmt.Errorf("audit: write %s: %w", entry.ID, err)
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, fmt.Errorf("audit: flush: %w", err)
	}
	return buf.Bytes(), nil
}

// formulaSafe prefixes the cells a spreadsheet would evaluate. The rule is the object export's: a leading
// =, +, -, @ or tab makes the cell a formula, so it is written as text instead.
func formulaSafe(cell string) string {
	if cell == "" {
		return ""
	}
	switch cell[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + cell
	}
	return cell
}

// Filename renders the audit export's name: the tenant, the first day the entries cover and the extension.
func Filename(tenant string, entries []Entry) string {
	day := "empty"
	if len(entries) > 0 && len(entries[0].At) >= 10 {
		day = entries[0].At[:10]
	}
	return "audit-" + tenant + "-" + day + ".csv"
}

// Counts renders how many entries each outcome carries, as the console's summary line.
func Counts(entries []Entry) map[string]string {
	counts := map[string]int{}
	for _, entry := range entries {
		counts[entry.Outcome]++
	}
	out := map[string]string{}
	for outcome, count := range counts {
		out[outcome] = strconv.Itoa(count)
	}
	return out
}
