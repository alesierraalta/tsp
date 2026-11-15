// Package batchdec decodes the row blocks partner feeds send to the ingest endpoint.
package batchdec

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// The limits a block is held to. They exist so a partner feed cannot make this package size work on its
// behalf: the header is refused when it asks for more than the block limits allow.
const (
	MaxColumns  = 64
	MaxDeclared = 100_000
	MaxRepeat   = 1_000
)

// ErrFormat reports a block this package cannot read.
var ErrFormat = errors.New("batchdec: malformed block")

// ErrTooMany reports a count above the limit the format allows.
var ErrTooMany = errors.New("batchdec: count above the block limit")

// Row is one decoded row of a block.
type Row struct {
	Label string
	Value int
}

// Meta is the header of a block.
type Meta struct {
	Version  int
	Columns  int
	Declared int
}

// ParseHeader reads a block header: "v1 cols=3 rows=2".
func ParseHeader(line string) (Meta, error) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return Meta{}, fmt.Errorf("%w: empty header", ErrFormat)
	}
	version, ok := strings.CutPrefix(fields[0], "v")
	if !ok {
		return Meta{}, fmt.Errorf("%w: header %q does not start with a version", ErrFormat, line)
	}
	major, err := strconv.Atoi(version)
	if err != nil || major != 1 {
		return Meta{}, fmt.Errorf("%w: unsupported version %q", ErrFormat, version)
	}
	meta := Meta{Version: major}
	for _, field := range fields[1:] {
		key, value, ok := strings.Cut(field, "=")
		if !ok {
			return Meta{}, fmt.Errorf("%w: header field %q is not key=value", ErrFormat, field)
		}
		number, err := strconv.Atoi(value)
		if err != nil {
			return Meta{}, fmt.Errorf("%w: header field %q is not a number", ErrFormat, field)
		}
		if number < 0 {
			return Meta{}, fmt.Errorf("%w: header field %q is negative", ErrFormat, field)
		}
		switch key {
		case "cols":
			meta.Columns = number
		case "rows":
			meta.Declared = number
		default:
			return Meta{}, fmt.Errorf("%w: unknown header field %q", ErrFormat, key)
		}
	}
	if meta.Columns == 0 || meta.Columns > MaxColumns {
		return Meta{}, fmt.Errorf("%w: cols=%d, limit %d", ErrTooMany, meta.Columns, MaxColumns)
	}
	if meta.Declared > MaxDeclared {
		return Meta{}, fmt.Errorf("%w: rows=%d, limit %d", ErrTooMany, meta.Declared, MaxDeclared)
	}
	return meta, nil
}

// ParseRow reads one row line: "label|value" with an optional " x<repeat>" suffix.
func ParseRow(line string) (Row, int, error) {
	body, repeat := line, 1
	if index := strings.LastIndex(line, " x"); index >= 0 {
		body = line[:index]
		number, err := strconv.Atoi(line[index+2:])
		if err != nil {
			return Row{}, 0, fmt.Errorf("%w: repeat factor of %q is not a number", ErrFormat, line)
		}
		if number < 1 {
			return Row{}, 0, fmt.Errorf("%w: repeat factor of %q is below one", ErrFormat, line)
		}
		repeat = number
	}
	label, value, ok := strings.Cut(body, "|")
	if !ok {
		return Row{}, 0, fmt.Errorf("%w: row %q has no value column", ErrFormat, line)
	}
	if strings.TrimSpace(label) == "" {
		return Row{}, 0, fmt.Errorf("%w: row %q has no label", ErrFormat, line)
	}
	number, err := strconv.Atoi(value)
	if err != nil {
		return Row{}, 0, fmt.Errorf("%w: row %q has a non-numeric value", ErrFormat, line)
	}
	return Row{Label: label, Value: number}, repeat, nil
}

// Decode returns the rows of one block, in the order the block lists them.
func Decode(body string) ([]Row, error) {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
		return nil, fmt.Errorf("%w: no header", ErrFormat)
	}
	meta, err := ParseHeader(lines[0])
	if err != nil {
		return nil, err
	}
	rows := make([]Row, 0, meta.Declared)
	declared := 0
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		row, repeat, err := ParseRow(line)
		if err != nil {
			return nil, err
		}
		declared++
		for i := 0; i < repeat; i++ {
			rows = append(rows, row)
		}
	}
	if declared != meta.Declared {
		return nil, fmt.Errorf("%w: header declares %d rows, block carries %d", ErrFormat, meta.Declared, declared)
	}
	return rows, nil
}

// Encode writes rows back out as a block, collapsing runs of one label and value into a repeat factor. The
// header counts the row lines it writes, which is what Decode reads back.
func Encode(rows []Row) string {
	var body strings.Builder
	lines := 0
	for i := 0; i < len(rows); {
		run := 1
		for i+run < len(rows) && rows[i+run] == rows[i] && run < MaxRepeat {
			run++
		}
		body.WriteString(rows[i].Label)
		body.WriteString("|")
		body.WriteString(strconv.Itoa(rows[i].Value))
		if run > 1 {
			body.WriteString(" x")
			body.WriteString(strconv.Itoa(run))
		}
		body.WriteString("\n")
		lines++
		i += run
	}
	return "v1 cols=2 rows=" + strconv.Itoa(lines) + "\n" + body.String()
}

// Summary totals the rows per label, in label order.
func Summary(rows []Row) []LabelTotal {
	totals := map[string]int{}
	for _, row := range rows {
		totals[row.Label] += row.Value
	}
	out := make([]LabelTotal, 0, len(totals))
	for label, value := range totals {
		out = append(out, LabelTotal{Label: label, Value: value})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Label < out[j].Label })
	return out
}

// LabelTotal is one line of a Summary.
type LabelTotal struct {
	Label string
	Value int
}
