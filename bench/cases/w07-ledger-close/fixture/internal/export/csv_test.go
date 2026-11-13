package export

import (
	"encoding/csv"
	"strings"
	"testing"
	"time"

	"example.com/ledgerclose/internal/usage"
)

func TestSafeCellPrefixesWhatASpreadsheetWouldRun(t *testing.T) {
	cases := map[string]string{
		"=SUM(A1)":   "'=SUM(A1)",
		"+1+1":       "'+1+1",
		"-2+3":       "'-2+3",
		"@SUM(A1)":   "'@SUM(A1)",
		" =SUM(A1)":  "' =SUM(A1)",
		"\t=SUM(A1)": "'\t=SUM(A1)",
		"plain text": "plain text",
		"":           "",
		"mid=sign":   "mid=sign",
	}
	for raw, want := range cases {
		if got := SafeCell(raw); got != want {
			t.Fatalf("SafeCell(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestSafeCellBoundsACell(t *testing.T) {
	long := strings.Repeat("x", MaxCellBytes+100)
	if got := SafeCell(long); len(got) != MaxCellBytes {
		t.Fatalf("SafeCell kept %d bytes", len(got))
	}
}

func TestUsageWritesAQuotedReport(t *testing.T) {
	events := []usage.Event{
		{
			ID: "evt-0001-abcd", Account: "=acme", Metric: "api_call", Quantity: 10,
			UnitAmount: 100, Currency: "USD", OccurredAt: time.Date(2024, 5, 10, 12, 0, 0, 0, time.UTC),
		},
	}
	body, err := Usage(events)
	if err != nil {
		t.Fatalf("usage: %v", err)
	}
	records, err := csv.NewReader(strings.NewReader(string(body))).ReadAll()
	if err != nil {
		t.Fatalf("the report is not valid CSV: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("the report carries %d rows", len(records))
	}
	if records[0][0] != "id" || records[0][6] != "occurred_at" {
		t.Fatalf("header = %v", records[0])
	}
	if records[1][1] != "'=acme" {
		t.Fatalf("the account cell = %q", records[1][1])
	}
	if records[1][4] != "1.00" {
		t.Fatalf("the unit amount cell = %q", records[1][4])
	}
	if records[1][6] != "2024-05-10T12:00:00Z" {
		t.Fatalf("the instant cell = %q", records[1][6])
	}
}
