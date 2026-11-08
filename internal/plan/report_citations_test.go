package plan

import (
	"strconv"
	"strings"
	"testing"
)

const reportFindingsHeader = "## Findings\n\n| Id | Finding (path:line, one line) | Severity | Data safe? | Evidence id | Pinning test | Status | Verdict by / date | Reason | Fingerprint |\n|---|---|---|---|---|---|---|---|---|---|\n"

func reportPlan(rows ...string) string {
	return "# Test plan\n\n" + reportFindingsHeader + strings.Join(rows, "\n") + "\n\n## Evidence ledger\n"
}

func findingRow(finding string) string {
	return "| F1 | " + finding + " | high | yes | - | - | confirmed | a / 2026-11-07 | r | - |"
}

// The shape of the study's reports (#372): the defect named in prose with its path:line, in Spanish.
const studyReport = "Lo pinné con un test que falla hoy.\n\n**Defecto:** `middleware/decompress.go:85` compara `Content-Encoding` sin ignorar mayúsculas.\n"

func cited(cs []string) string { return strings.Join(cs, ",") }

func unrecorded(planText, report string) []string {
	var out []string
	for _, c := range UnrecordedCitations(planText, report) {
		out = append(out, c.Path+":"+strconv.Itoa(c.First))
	}
	return out
}

func TestUnrecordedCitationsNamesADefectTheReportCitesAndThePlanDoesNot(t *testing.T) {
	if got := unrecorded(reportPlan(), studyReport); cited(got) != "middleware/decompress.go:85" {
		t.Fatalf("unrecorded = %v, want the report's citation", got)
	}
	if got := unrecorded("# Test plan\n\nno findings section\n", studyReport); cited(got) != "middleware/decompress.go:85" {
		t.Fatalf("a plan without Findings records nothing, got %v", got)
	}
}

func TestUnrecordedCitationsAcceptsARowThatRecordsTheDefect(t *testing.T) {
	for name, finding := range map[string]string{
		"same line":          "middleware/decompress.go:85 case-sensitive encoding",
		"within tolerance":   "middleware/decompress.go:83 case-sensitive encoding",
		"range covering it":  "middleware/decompress.go:80-90 encoding compare",
		"file without line":  "decompress.go compares the encoding case-sensitively",
		"suffix of the path": "decompress.go:85 encoding",
	} {
		if got := unrecorded(reportPlan(findingRow(finding)), studyReport); len(got) != 0 {
			t.Errorf("%s: unrecorded = %v, want none", name, got)
		}
	}
	if got := unrecorded(reportPlan(findingRow("middleware/decompress.go:40 something else")), studyReport); cited(got) != "middleware/decompress.go:85" {
		t.Errorf("a row far from the cited line must not record it, got %v", got)
	}
}

func TestUnrecordedCitationsIgnoresTestsDocsAndCitationsWithoutALine(t *testing.T) {
	report := "Tests in `middleware/decompress_test.go:40`, `src/a.test.js:3`, `tests/test_x.py:9`, `testdata/in.json:2`; docs in `README.md:3`; touched `middleware/decompress.go`."
	if got := unrecorded(reportPlan(), report); len(got) != 0 {
		t.Fatalf("unrecorded = %v, want none", got)
	}
}

func TestUnrecordedCitationsMatchesALongPathBySuffixAndReportsEachOnce(t *testing.T) {
	report := "**Defecto:** `cmd/jaeger/internal/extension/jaegerquery/internal/mcptools/internal/handlers/search_traces.go:168`.\nAgain: cmd/jaeger/internal/extension/jaegerquery/internal/mcptools/internal/handlers/search_traces.go:168 and `pkg/compose/relay.go:223`."
	got := unrecorded(reportPlan(findingRow("search_traces.go:168 depth unlimited")), report)
	if cited(got) != "pkg/compose/relay.go:223" {
		t.Fatalf("unrecorded = %v, want only the relay citation, once", got)
	}
	got = unrecorded(reportPlan(), report)
	if len(got) != 2 {
		t.Fatalf("unrecorded = %v, want two distinct citations", got)
	}
}
