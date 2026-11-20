package telemetry

import (
	"bytes"
	"encoding/csv"
	"strings"
	"testing"
)

func TestExportJSONLKeepsValidLinesVerbatim(t *testing.T) {
	path := writeStatsLedger(t, statsFixture)
	var out bytes.Buffer
	result, err := ExportJSONL(&out, path)
	if err != nil {
		t.Fatal(err)
	}
	if result.Valid != 10 || result.Malformed != 1 {
		t.Fatalf("export result = %+v, want 10 valid / 1 malformed", result)
	}
	want := strings.Join(statsFixture[:10], "\n") + "\n"
	if out.String() != want {
		t.Fatalf("jsonl export:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestExportCSVFlattensDottedFieldsWithSortedHeader(t *testing.T) {
	path := writeStatsLedger(t, []string{
		`{"schema":1,"source":"tsp","kind":"command","command":{"name":"check","flags":["path","run"],"exit":2,"duration_ms":7}}`,
		`{"schema":1,"source":"pi","kind":"host_run","host":{"tokens":{"input":10},"tools":{"bash":2,"read":3}}}`,
	})
	var out bytes.Buffer
	result, err := ExportCSV(&out, path)
	if err != nil {
		t.Fatal(err)
	}
	if result.Valid != 2 || result.Malformed != 0 {
		t.Fatalf("export result = %+v", result)
	}
	rows, err := csv.NewReader(strings.NewReader(out.String())).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("csv rows = %d, want header + 2: %q", len(rows), out.String())
	}
	header := rows[0]
	wantHeader := []string{
		"command.duration_ms", "command.exit", "command.flags", "command.name",
		"host.tokens.input", "host.tools", "kind", "schema", "source",
	}
	if strings.Join(header, ",") != strings.Join(wantHeader, ",") {
		t.Fatalf("header = %v, want %v", header, wantHeader)
	}
	cell := func(row int, name string) string {
		for i, column := range header {
			if column == name {
				return rows[row][i]
			}
		}
		t.Fatalf("no column %q in %v", name, header)
		return ""
	}
	if got := cell(1, "command.flags"); got != `["path","run"]` {
		t.Errorf("flags cell = %q, want JSON array", got)
	}
	if got := cell(1, "command.duration_ms"); got != "7" {
		t.Errorf("duration cell = %q, want 7", got)
	}
	if got := cell(2, "host.tools"); got != `{"bash":2,"read":3}` {
		t.Errorf("tools cell = %q, want JSON map", got)
	}
	if got := cell(1, "host.tokens.input"); got != "" {
		t.Errorf("missing cell = %q, want empty", got)
	}
}
