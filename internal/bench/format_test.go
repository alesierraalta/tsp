package bench

import (
	"github.com/alesierraalta/tsp/internal/citation"
	"strings"
	"testing"
)

// A plan whose findings are prose scores zero the same way an empty plan does. The two are
// different failures and the run must say which one happened.
func TestPlanFormatIsReported(t *testing.T) {
	key := Key{ID: "c", Defects: []Defect{{ID: "D1", File: "src/a.js", Line: 5, Keywords: []string{"x"}}}}
	ledger := "| E1 | c | cmd | i | o | m | r | observado |\n"
	cases := []struct {
		name     string
		plan     string
		want     string
		wantNote string
	}{
		{
			name: "rows in the template table",
			plan: plan("| F1 | src/a.js:5 x | M | yes | E1 | open | me | - | - |\n", ledger),
			want: FormatTable,
		},
		{
			name: "no findings at all",
			plan: plan("", ledger),
			want: FormatEmpty,
		},
		{
			name: "placeholder rows only",
			plan: plan("| (none) | | | | | | | | |\n", ledger),
			want: FormatEmpty,
		},
		{
			name: "findings written as prose sections",
			plan: "# Test plan\n\n## Findings\n\n### F1 — the parser drops a quoted comma\n" +
				"- Severity: data loss\n- Pinning test: `tests/csv.test.js :: keeps a quoted comma`\n" +
				"- Status: fixed\n\n## Evidence ledger\n\n" + ledgerHeader + ledger,
			want:     FormatProse,
			wantNote: "not in the template table",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := Score(tc.plan, key)
			if r.PlanFormat != tc.want {
				t.Fatalf("format = %q, want %q", r.PlanFormat, tc.want)
			}
			if tc.wantNote != "" && !strings.Contains(strings.Join(r.Notes, " "), tc.wantNote) {
				t.Fatalf("notes = %v", r.Notes)
			}
		})
	}
}

// A finding that names no file cannot be located, whatever else it says.
func TestFindingsWithoutAPathAreReported(t *testing.T) {
	key := Key{ID: "c", Defects: []Defect{{ID: "D1", File: "src/ledger.js", Line: 11, Keywords: []string{"round"}}}}
	ledger := "| E1 | c | node -e probe calling formatCents | i | o | m | r | observado |\n"
	r := Score(plan("| F1 | `formatCents` rounds a tie down | M | yes | E1 | open | me | - | - |\n", ledger), key)
	if r.Found != 0 {
		t.Fatalf("a finding with no path must not match: %+v", r.Defects)
	}
	if r.RowsWithoutPath != 1 {
		t.Fatalf("rows without a path = %d, want 1", r.RowsWithoutPath)
	}
	if !strings.Contains(strings.Join(r.Notes, " "), "cite no file") {
		t.Fatalf("notes = %v", r.Notes)
	}
}

// A decimal number is not a file. Reading "1.005" as a path made every row look located.
func TestCitationsIgnoreNumbers(t *testing.T) {
	got := citation.Extract("`formatCents` rounds `1.005 * 100` to 100.49999999999999 in src/ledger.js:11")
	if len(got) != 1 || got[0].Path != "src/ledger.js" || got[0].First != 11 {
		t.Fatalf("citations = %+v", got)
	}
	if c := citation.Extract("rounds 1.005 to 1.00"); len(c) != 0 {
		t.Fatalf("numbers read as paths: %+v", c)
	}
}
