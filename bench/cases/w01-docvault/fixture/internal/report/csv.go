package report

import (
	"encoding/csv"
	"strconv"
	"strings"

	"example.com/docvault/internal/vault"
)

// Formula prefixes a spreadsheet evaluates as an expression. A cell that begins with one is a
// formula the export would run when an operator opens it, so every exported cell passes through
// Cell before it is written.
const formulaPrefixes = "=+-@\t\r"

// Cell returns a spreadsheet-safe rendering of a value. A value that a spreadsheet would evaluate
// is prefixed with an apostrophe, which is what the export contract promises: the sheet shows the
// text, never the formula.
func Cell(value string) string {
	if value == "" {
		return ""
	}
	if strings.ContainsRune(formulaPrefixes, rune(value[0])) {
		return "'" + value
	}
	return value
}

// Header is the column list of the document export.
var Header = []string{"id", "key", "tenant", "kind", "owner", "title", "tags", "bytes", "updated"}

// CSV renders a document set as CSV, one row per document, every cell through Cell.
func CSV(docs []vault.Doc) (string, error) {
	var b strings.Builder
	w := csv.NewWriter(&b)
	if err := w.Write(Header); err != nil {
		return "", err
	}
	for _, d := range docs {
		row := []string{
			Cell(d.ID),
			Cell(d.Key),
			Cell(d.Tenant),
			Cell(d.Kind),
			Cell(d.Owner),
			Cell(d.Title),
			Cell(strings.Join(d.Tags, " ")),
			strconv.FormatInt(d.Size, 10),
			d.Updated.UTC().Format("2006-01-02T15:04:05Z"),
		}
		if err := w.Write(row); err != nil {
			return "", err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return "", err
	}
	return b.String(), nil
}

// Rows renders a document set as a slice of rows, for callers that hand the export to another
// encoder. It applies the same cell rule as CSV, so a second encoder cannot reintroduce what the
// first one defused.
func Rows(docs []vault.Doc) [][]string {
	rows := make([][]string, 0, len(docs))
	for _, d := range docs {
		rows = append(rows, []string{
			Cell(d.ID),
			Cell(d.Tenant),
			Cell(d.Kind),
			Cell(d.Title),
			strconv.FormatInt(d.Size, 10),
		})
	}
	return rows
}
