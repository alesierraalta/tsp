package telemetry

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"io"
	"sort"
	"strconv"
	"strings"
)

// ExportResult says how many lines an export carried and how many malformed lines it skipped. A
// malformed line is never silently dropped: the caller reports the count on stderr.
type ExportResult struct {
	Valid     int
	Malformed int
}

// ExportJSONL writes every valid ledger line verbatim, in ledger order. Malformed lines are skipped
// and counted.
func ExportJSONL(w io.Writer, path string) (ExportResult, error) {
	lines, malformed, err := readLedger(path)
	if err != nil {
		return ExportResult{}, err
	}
	writer := bufioWriter(w)
	defer writer.Flush()
	for _, line := range lines {
		if _, err := writer.Write(append(line.raw, '\n')); err != nil {
			return ExportResult{}, err
		}
	}
	return ExportResult{Valid: len(lines), Malformed: malformed}, nil
}

// ExportCSV writes one column per flattened dotted field, header sorted so the same ledger always
// yields the same columns. A nested object is flattened; an array or a dynamic map (host.tools) is
// JSON-encoded in its cell; a field a line did not measure is empty.
func ExportCSV(w io.Writer, path string) (ExportResult, error) {
	lines, malformed, err := readLedger(path)
	if err != nil {
		return ExportResult{}, err
	}
	rows := make([]map[string]string, 0, len(lines))
	columns := map[string]struct{}{}
	for _, line := range lines {
		row := map[string]string{}
		flatten("", line.fields, row)
		for column := range row {
			columns[column] = struct{}{}
		}
		rows = append(rows, row)
	}
	header := make([]string, 0, len(columns))
	for column := range columns {
		header = append(header, column)
	}
	sort.Strings(header)

	writer := csv.NewWriter(w)
	if err := writer.Write(header); err != nil {
		return ExportResult{}, err
	}
	record := make([]string, len(header))
	for _, row := range rows {
		for i, column := range header {
			record[i] = row[column]
		}
		if err := writer.Write(record); err != nil {
			return ExportResult{}, err
		}
	}
	writer.Flush()
	return ExportResult{Valid: len(lines), Malformed: malformed}, writer.Error()
}

// flatten walks a generically decoded object into dotted leaf names.
func flatten(prefix string, value any, row map[string]string) {
	switch typed := value.(type) {
	case map[string]any:
		if isMapLeaf(prefix) || len(typed) == 0 {
			row[prefix] = jsonCell(typed)
			return
		}
		for key, child := range typed {
			name := key
			if prefix != "" {
				name = prefix + "." + key
			}
			flatten(name, child, row)
		}
	case []any:
		row[prefix] = jsonCell(typed)
	default:
		row[prefix] = scalarCell(typed)
	}
}

// isMapLeaf names the dynamic maps the schema carries: their keys are data, not columns.
func isMapLeaf(prefix string) bool {
	return prefix == "tools" || strings.HasSuffix(prefix, ".tools")
}

// scalarCell renders a JSON scalar as an empty cell when it is null.
func scalarCell(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case json.Number:
		return typed.String()
	case bool:
		return strconv.FormatBool(typed)
	case float64:
		return strconv.FormatFloat(typed, 'g', -1, 64)
	case int64:
		return strconv.FormatInt(typed, 10)
	default:
		return jsonCell(typed)
	}
}

// jsonCell encodes an array or a dynamic map as the cell's JSON text.
func jsonCell(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(data)
}

// bufioWriter keeps the export to one buffered pass over the ledger.
func bufioWriter(w io.Writer) *bufio.Writer {
	return bufio.NewWriter(w)
}
