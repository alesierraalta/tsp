package plan

import (
	"fmt"
	"path"
	"strings"

	"github.com/alesierraalta/tsp/internal/citation"
)

// reportLineTolerance is how far a Findings row's line may sit from the line a report cites and still record it:
// the same tolerance the bench scorer gives a finding against its defect.
const reportLineTolerance = 5

// UnrecordedCitations returns every production path:line the report cites that no Findings row records, once each
// and in report order. A row records a citation when it cites the same file (by path suffix) at a line within the
// tolerance, over a range that covers it, or with no line at all. Citations of test files, of documentation, and
// without a line are not findings and are never returned. A defect that lives only in the report is lost to every
// reader of the plan, which is why a report that cites one is incomplete.
func UnrecordedCitations(planText, report string) []citation.Citation {
	var recorded []citation.Citation
	_, rows := Table(planText, "Findings")
	for _, r := range rows {
		if len(r) > 1 {
			recorded = append(recorded, citation.Extract(r[1])...)
		}
	}
	seen := map[string]bool{}
	var out []citation.Citation
	for _, c := range citation.Extract(report) {
		key := fmt.Sprintf("%s:%d", c.Path, c.First)
		if c.First == 0 || seen[key] || !productionFile(c.Path) || recordedBy(recorded, c) {
			continue
		}
		seen[key] = true
		out = append(out, c)
	}
	return out
}

func recordedBy(rows []citation.Citation, c citation.Citation) bool {
	for _, r := range rows {
		if !citation.SamePath(r.Path, c.Path) {
			continue
		}
		if r.First == 0 || (c.First >= r.First-reportLineTolerance && c.First <= r.Last+reportLineTolerance) {
			return true
		}
	}
	return false
}

// productionFile reports whether a cited path is code a finding can be about: not a test, not a test fixture,
// and not documentation.
func productionFile(p string) bool {
	p = strings.ToLower(strings.ReplaceAll(p, "\\", "/"))
	base := path.Base(p)
	switch path.Ext(base) {
	case ".md", ".txt", ".rst":
		return false
	}
	for _, seg := range strings.Split(path.Dir(p), "/") {
		switch seg {
		case "test", "tests", "__tests__", "testdata":
			return false
		}
	}
	stem := strings.TrimSuffix(base, path.Ext(base))
	return !(strings.HasSuffix(stem, "_test") || strings.HasPrefix(stem, "test_") ||
		strings.HasSuffix(stem, ".test") || strings.HasSuffix(stem, ".spec"))
}
