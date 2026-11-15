package intake

import (
	"fmt"
	"sort"
	"strings"

	"example.com/archive/internal/checksum"
	"example.com/archive/internal/manifest"
	"example.com/archive/internal/textutil"
)

// EntryReport is the report's view of one entry: the manifest value, the pipeline's result and the
// classification, together in one row.
type EntryReport struct {
	Classification Classification
	Result         EntryResult
}

// Reported pairs every entry of a manifest with its result and classification. An entry the result does
// not mention is reported as missing rather than dropped: a batch report that silently loses an entry is
// a report an operator cannot trust.
func Reported(m manifest.Manifest, result Result) []EntryReport {
	byName := map[string]EntryResult{}
	for _, entry := range result.Entries {
		byName[entry.Name] = entry
	}
	classes := ClassifyAll(m)
	reports := make([]EntryReport, 0, len(m.Entries))
	for i, e := range m.Entries {
		outcome, ok := byName[e.Name]
		if !ok {
			outcome = EntryResult{Name: e.Name, Kind: e.KindOf(), Err: "the pipeline reported no outcome"}
		}
		reports = append(reports, EntryReport{Classification: classes[i], Result: outcome})
	}
	sort.SliceStable(reports, func(i, j int) bool { return reports[i].Result.Name < reports[j].Result.Name })
	return reports
}

// Rows renders the entry reports as the table rows the console prints: name, kind, pages, bytes, digest and
// the outcome.
func Rows(reports []EntryReport) [][]string {
	rows := make([][]string, 0, len(reports))
	for _, report := range reports {
		outcome := "committed"
		switch {
		case report.Result.Err != "":
			outcome = "failed: " + report.Result.Err
		case report.Result.Warn != "":
			outcome = report.Result.Warn
		}
		rows = append(rows, []string{
			report.Result.Name,
			report.Classification.Kind,
			fmt.Sprintf("%d", report.Result.Pages),
			textutil.Bytes(report.Result.Bytes),
			report.Result.Digest,
			outcome,
		})
	}
	return rows
}

// Counters is the report's totals: committed, failed, pages and bytes, computed from the entry reports.
type Counters struct {
	Entries   int
	Committed int
	Failed    int
	Warned    int
	Pages     int
	Bytes     int64
	Digests   int
}

// Count reads the counters from a manifest and the reports built for it.
func Count(m manifest.Manifest, reports []EntryReport) Counters {
	counters := Counters{Entries: len(reports)}
	digests := map[string]bool{}
	for _, report := range reports {
		switch {
		case report.Result.Err != "":
			counters.Failed++
		case report.Result.Warn != "":
			counters.Warned++
			counters.Committed++
		default:
			counters.Committed++
		}
		counters.Pages += report.Result.Pages
		counters.Bytes += report.Result.Bytes
		if report.Result.Digest != "" {
			digests[report.Result.Digest] = true
		}
	}
	counters.Digests = len(digests)
	return counters
}

// Describe renders the counters as the report's summary line.
func (c Counters) Describe() string {
	parts := []string{
		fmt.Sprintf("%d entries", c.Entries),
		fmt.Sprintf("%d committed", c.Committed),
	}
	if c.Failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", c.Failed))
	}
	if c.Warned > 0 {
		parts = append(parts, fmt.Sprintf("%d warned", c.Warned))
	}
	parts = append(parts, fmt.Sprintf("%d pages", c.Pages), textutil.Bytes(c.Bytes))
	return strings.Join(parts, ", ")
}

// DigestLines renders the digest of every entry, which is what an operator compares against the sender's
// own manifest.
func DigestLines(m manifest.Manifest, reports []EntryReport) []string {
	var lines []string
	for _, report := range reports {
		digest := report.Result.Digest
		if digest == "" {
			digest = "(not committed)"
		}
		declared := ""
		if entry, ok := m.EntryByName(report.Result.Name); ok && entry.Digest != "" {
			declared = checksum.Short(entry.Digest)
		}
		if declared != "" {
			lines = append(lines, fmt.Sprintf("%s %s declared %s", report.Result.Name, digest, declared))
			continue
		}
		lines = append(lines, report.Result.Name+" "+digest)
	}
	return lines
}
