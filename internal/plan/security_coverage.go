package plan

import (
	"fmt"
	"strings"
)

// securityCoverageName is the optional section a plan uses to say what security surface, boundary,
// subsystem or attack class a run covered, and — for the rest — why it did not. Plans written before
// it existed keep their meaning; when it is present, the same cells feed CheckDocument and Gaps, so the
// checker and the report cannot disagree about what a row says.
const securityCoverageName = "Security coverage"

// coverageKinds is the closed vocabulary a coverage row uses to name what it is about. One row names
// one subject: a surface, a boundary, a subsystem, or an attack class that applies to this system.
var coverageKinds = map[string]bool{
	"surface": true, "boundary": true, "subsystem": true, "attack class": true,
}

// coverageDispositions is the four-state vocabulary the issue asks for. Only `covered` claims work
// done, and it owes observed evidence; `excluded` is a reasoned decision, not an omission, so it owes
// no coverage. `blocked` and `deferred` stay owed, which is what makes an incomplete sweep reportable.
var coverageDispositions = map[string]bool{
	"covered": true, "blocked": true, "excluded": true, "deferred": true,
}

// coverageColumns are the cells every coverage row owes. `Reason` is checked per disposition, because
// a covered row may legitimately carry `-` while a blocked one cannot.
var coverageColumns = []string{"id", "source", "kind", "subject", "disposition", "reason", "evidence id"}

// securityCoverageProblems validates the optional coverage contract. It checks that each row can be
// located, names what it covers and says honestly which of the four states it is in; it does not claim
// an observation is true, exactly as the candidate contract does not.
func securityCoverageProblems(lines []string, ledger tableScan) []string {
	present, table, columns, problems := coverageSectionProblems(lines)
	if !present || table.header == nil || columns == nil {
		return problems
	}
	return append(problems, coverageRowsProblems(table.rows, columns, observedLedgerRows(ledger))...)
}

// coverageSectionProblems reads the section structure once for both callers, through the fence-aware
// locator: a documentation example is never a section, and the first real section is the one read.
// `columns` is nil when the header is missing a column the row rules read, because a row read through a
// missing column is not a row this package can judge. Everything the table shape hides from scanTable —
// a second table, a populated row whose placeholder id the row reader skips — is reported here, so a
// caller that never runs CheckDocument still fails closed.
func coverageSectionProblems(lines []string) (present bool, table tableScan, columns map[string]int, problems []string) {
	heading, end, regions := securityCoverageRegion(lines)
	if heading < 0 {
		return false, tableScan{}, nil, nil
	}
	present = true
	if regions > 1 {
		problems = append(problems, "Security coverage section is repeated")
	}
	table = scanTable(lines, heading+1, end)
	if table.header == nil {
		return present, table, nil, append(problems, fmt.Sprintf("line %d: Security coverage has no table", heading+1))
	}
	problems = append(problems, coverageTableShapeProblems(lines, heading+1, end)...)
	if line, message := interruptedTable(securityCoverageName, table); message != "" {
		problems = append(problems, fmt.Sprintf("line %d: %s", line, message))
	}
	columns, columnProblems := coverageColumnIndexes(table)
	problems = append(problems, columnProblems...)
	if len(columnProblems) != 0 {
		return present, table, nil, problems
	}
	return present, table, columns, problems
}

// coverageColumnIndexes resolves the columns the row rules read and names every repeated or missing one.
func coverageColumnIndexes(table tableScan) (map[string]int, []string) {
	var problems []string
	columns := map[string]int{}
	for i, header := range table.header {
		name := strings.ToLower(strings.TrimSpace(header))
		if _, exists := columns[name]; exists {
			problems = append(problems, fmt.Sprintf("line %d: Security coverage repeats column %s", table.head, quote(name)))
		}
		columns[name] = i
	}
	for _, name := range coverageColumns {
		if _, exists := columns[name]; !exists {
			problems = append(problems, fmt.Sprintf("line %d: Security coverage has no %s column", table.head, name))
		}
	}
	if len(problems) != 0 {
		return nil, problems
	}
	return columns, nil
}

// coverageRowsProblems reads every coverage row against the rules CheckDocument enforces.
func coverageRowsProblems(rows []row, columns map[string]int, ledgerRows map[string]bool) []string {
	var problems []string
	seen := map[string]int{}
	for _, r := range rows {
		problems = append(problems, coverageRowProblem(r, columns, ledgerRows, seen)...)
	}
	return problems
}

// coverageRowProblem reads one coverage row. Gaps calls the same rules, so a row the checker would refuse
// cannot be counted as covered by a report the checker never saw.
func coverageRowProblem(r row, columns map[string]int, ledgerRows map[string]bool, seen map[string]int) []string {
	value := func(name string) string { return cell(r.cells, columns[name]) }
	id := value("id")
	var problems []string
	if first, exists := seen[id]; exists {
		problems = append(problems, fmt.Sprintf("line %d: security coverage %s repeats line %d", r.line, quote(id), first))
	} else {
		seen[id] = r.line
	}
	if !pathCiteRe.MatchString(value("source")) {
		problems = append(problems, fmt.Sprintf("line %d: security coverage %s cites no path:line in Source, so the work it covers cannot be located", r.line, quote(id)))
	}
	if !coverageKinds[strings.ToLower(value("kind"))] {
		problems = append(problems, fmt.Sprintf("line %d: security coverage %s kind must be surface, boundary, subsystem, or attack class", r.line, quote(id)))
	}
	if placeholder.MatchString(value("subject")) {
		problems = append(problems, fmt.Sprintf("line %d: security coverage %s requires a subject", r.line, quote(id)))
	}
	disposition := strings.ToLower(value("disposition"))
	if !coverageDispositions[disposition] {
		return append(problems, fmt.Sprintf("line %d: security coverage %s disposition must be covered, blocked, excluded, or deferred", r.line, quote(id)))
	}
	if disposition != "covered" && placeholder.MatchString(value("reason")) {
		problems = append(problems, fmt.Sprintf("line %d: security coverage %s %s requires a reason", r.line, quote(id), disposition))
	}
	return append(problems, coverageEvidenceProblems(r, id, value("evidence id"), ledgerRows, disposition == "covered")...)
}

// observedLedgerRows is the ledger view both callers judge a coverage citation against: an id maps to
// whether that row can carry a coverage claim at all.
func observedLedgerRows(ledger tableScan) map[string]bool {
	observed := map[string]bool{}
	for _, r := range ledger.rows {
		observed[cell(r.cells, 0)] = observedLedgerRow(r, ledger.header)
	}
	return observed
}

// coverageEvidenceProblems holds a covered row to observed evidence, because a coverage claim nobody
// observed is the one shape that can turn an incomplete sweep into a complete-looking report. Any other
// disposition may cite evidence too, and then it has to resolve like every other citation.
func coverageEvidenceProblems(r row, id, raw string, ledgerRows map[string]bool, required bool) []string {
	evidence := strings.Trim(raw, "`")
	if placeholder.MatchString(evidence) {
		if required {
			return []string{fmt.Sprintf("line %d: security coverage %s is covered but cites no observed Evidence ledger row", r.line, quote(id))}
		}
		return nil
	}
	observed, exists := ledgerRows[evidence]
	if !exists {
		return []string{fmt.Sprintf("line %d: security coverage %s cites evidence %s, which is not a row in the Evidence ledger", r.line, quote(id), quote(evidence))}
	}
	if required && !observed {
		return []string{fmt.Sprintf("line %d: security coverage %s requires evidence %s with executed and observed details labelled observado", r.line, quote(id), quote(evidence))}
	}
	return nil
}

// observedLedgerRow reports whether one ledger row can carry a coverage claim: the label is the literal
// `observado`, and the prose cells that make the observation usable are filled. The header is the ledger's
// own, because the same rule has to read the shipped template's header and the older eight-column one.
func observedLedgerRow(r row, header []string) bool {
	filled := func(name string) bool {
		return !placeholder.MatchString(cell(r.cells, exactColumn(header, name)))
	}
	return strings.EqualFold(cell(r.cells, exactColumn(header, "label")), "observado") && filled("executed") && filled("observed")
}

// securityCoverageRegion ignores documentation fences and counts every actual section, so a repeated
// heading is reported instead of silently shadowing the first one.
func securityCoverageRegion(lines []string) (heading, end, count int) {
	heading, end = -1, len(lines)
	fence := ""
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if fence != "" {
			if strings.HasPrefix(t, fence) {
				fence = ""
			}
			continue
		}
		if marker := fenceMarker(t); marker != "" {
			fence = marker
			continue
		}
		if !strings.HasPrefix(t, "## ") {
			continue
		}
		if heading >= 0 && end == len(lines) {
			end = i
		}
		if strings.EqualFold(strings.TrimSpace(t[3:]), securityCoverageName) {
			count++
			if heading < 0 {
				heading = i
			}
		}
	}
	return
}

// coverageTableShapeProblems reads the region once for the shapes scanTable cannot report: a second
// table that starts legitimately under a subheading, and a populated row whose id is a placeholder —
// which the row reader skips, so without this pass the row would vanish from the count entirely.
func coverageTableShapeProblems(lines []string, start, end int) []string {
	var problems []string
	state, fence := 0, ""
	for i := start; i < end; i++ {
		t := strings.TrimSpace(lines[i])
		if fence != "" {
			if strings.HasPrefix(t, fence) {
				fence = ""
			}
			continue
		}
		if marker := fenceMarker(t); marker != "" {
			fence = marker
			continue
		}
		if t == "" {
			continue
		}
		if !strings.HasPrefix(t, "|") {
			if state == 1 {
				state = 2
			}
			continue
		}
		if state == 2 {
			problems = append(problems, fmt.Sprintf("line %d: Security coverage has more than one table", i+1))
			break
		}
		if state == 0 {
			state = 1
			continue
		}
		cells := split(t)
		if isSeparator(cells) || !placeholder.MatchString(cell(cells, 0)) {
			continue
		}
		for _, v := range cells[1:] {
			if !placeholder.MatchString(strings.TrimSpace(v)) {
				problems = append(problems, fmt.Sprintf("line %d: populated security coverage requires an id", i+1))
				break
			}
		}
	}
	return problems
}

// coverageGaps reads the same table GapsForRun's other passes read, but answers the coverage question:
// how much of the declared security work is covered, and which rows still owe an answer. A covered row
// counts only when its evidence resolves, so a plan the checker never saw cannot report unproven
// coverage as done. Excluded rows are reasoned decisions and leave the denominator, exactly as an
// `n/a` layer does.
func coverageGaps(g *Gaps, lines []string) {
	present, table, columns, structural := coverageSectionProblems(lines)
	if !present {
		return
	}
	g.CoverageProblems = append(g.CoverageProblems, structural...)
	if table.header == nil || columns == nil {
		return
	}
	observed := observedLedgerRows(scanSection(lines, "Evidence ledger"))
	iDisposition, iReason, iSubject := columns["disposition"], columns["reason"], columns["subject"]
	seen := map[string]int{}
	for _, r := range table.rows {
		if problems := coverageRowProblem(r, columns, observed, seen); len(problems) != 0 {
			// A row that cannot be read is owed, never covered: the problem names why, and Any() stays true.
			g.CoverageProblems = append(g.CoverageProblems, problems...)
			g.CoverageTotal++
			continue
		}
		id := quote(cell(r.cells, 0))
		subject := quote(cell(r.cells, iSubject))
		disposition := strings.ToLower(cell(r.cells, iDisposition))
		switch disposition {
		case "excluded":
			continue
		case "covered":
			g.CoverageTotal++
			g.CoverageCovered++
		case "blocked", "deferred":
			g.CoverageTotal++
			g.IncompleteCoverage = append(g.IncompleteCoverage, fmt.Sprintf("(line %d): %s %s — %s: %s", r.line, id, subject, disposition, coverageReason(cell(r.cells, iReason))))
		}
	}
}

// coverageReason renders the cell a blocked or deferred row uses to say what it is waiting for. An
// empty cell is named rather than printed as nothing, so the report never reads as complete-by-omission.
func coverageReason(raw string) string {
	if placeholder.MatchString(raw) {
		return "no reason recorded"
	}
	return quote(raw)
}
