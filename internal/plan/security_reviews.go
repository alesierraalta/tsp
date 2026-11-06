package plan

import (
	"fmt"
	"strings"
)

// securityReviewsName is the optional section where a reviewer other than the candidate's author records an
// attempt to refute it. It is deliberately separate from `## Security candidates`: a candidate row's
// Disposition is its author's claim, while a review row is someone else's answer to that claim. A candidate
// that carries a disposition keeps the T1 meaning whether or not this section exists.
const securityReviewsName = "Candidate reviews"

// independenceAttested is the one word that records an explicit independence attestation. It is a closed
// vocabulary because plan check can read a word; it cannot read a person, which is why the documentation says
// exactly what this cell does and does not prove.
const independenceAttested = "attested"

// securityReviewOutcomes is the closed vocabulary a review row uses to answer the candidate: confirm it, refute
// it, or retain a specific validation blocker that still has to be resolved.
var securityReviewOutcomes = map[string]bool{"confirmed": true, "rejected": true, "blocked": true}

// securityReviewColumns are the cells every review row owes. `Decisive evidence id` and `Blocker` are checked
// per outcome, because a confirmation without observed evidence is the one shape that turns an opinion into a
// confirmed claim, and a blocker without a name is not a validation step anyone can take.
var securityReviewColumns = []string{
	"id", "candidate id", "reviewer", "independence attestation", "refutation attempt",
	"effective controls", "outcome", "decisive evidence id", "blocker",
}

// securityReviewProblems validates the optional independent-review contract. It checks that each review names
// its candidate, its reviewer and an explicit attestation, records a refutation attempt and an evaluation of
// effective controls, and carries the evidence its outcome owes. It validates the recorded structure and
// references, not whether the reviewer was really independent or the evidence proves the claim.
func securityReviewProblems(lines []string, ledger tableScan) []string {
	present, table, columns, problems := reviewSectionProblems(lines)
	if !present || table.header == nil || columns == nil {
		return problems
	}
	return append(problems, reviewRowsProblems(lines, table.rows, columns, ledger)...)
}

// reviewSectionProblems reads the section structure once, through the fence-aware locator: a documentation
// example is never a section. `columns` is nil when the header is missing a column the row rules read, because
// a row read through a missing column is not a row this package can judge.
func reviewSectionProblems(lines []string) (present bool, table tableScan, columns map[string]int, problems []string) {
	heading, end, regions := securityReviewsRegion(lines)
	if heading < 0 {
		return false, tableScan{}, nil, nil
	}
	present = true
	if regions > 1 {
		problems = append(problems, "Candidate reviews section is repeated")
	}
	table = scanTable(lines, heading+1, end)
	if table.header == nil {
		return present, table, nil, append(problems, fmt.Sprintf("line %d: Candidate reviews has no table", heading+1))
	}
	problems = append(problems, reviewTableShapeProblems(lines, heading+1, end)...)
	if line, message := interruptedTable(securityReviewsName, table); message != "" {
		problems = append(problems, fmt.Sprintf("line %d: %s", line, message))
	}
	columns, columnProblems := reviewColumnIndexes(table)
	problems = append(problems, columnProblems...)
	if len(columnProblems) != 0 {
		return present, table, nil, problems
	}
	return present, table, columns, problems
}

// reviewColumnIndexes resolves the columns the row rules read and names every repeated or missing one.
func reviewColumnIndexes(table tableScan) (map[string]int, []string) {
	var problems []string
	columns := map[string]int{}
	for i, header := range table.header {
		name := strings.ToLower(strings.TrimSpace(header))
		if _, exists := columns[name]; exists {
			problems = append(problems, fmt.Sprintf("line %d: Candidate reviews repeats column %s", table.head, quote(name)))
		}
		columns[name] = i
	}
	for _, name := range securityReviewColumns {
		if _, exists := columns[name]; !exists {
			problems = append(problems, fmt.Sprintf("line %d: Candidate reviews has no %s column", table.head, name))
		}
	}
	if len(problems) != 0 {
		return nil, problems
	}
	return columns, nil
}

// reviewRowsProblems reads every review row against the rules CheckDocument enforces.
func reviewRowsProblems(lines []string, rows []row, columns map[string]int, ledger tableScan) []string {
	candidates := candidateIDs(lines)
	observed := observedLedgerRows(ledger)
	exists := map[string]bool{}
	for _, r := range ledger.rows {
		exists[cell(r.cells, 0)] = true
	}
	seen := map[string]int{}
	var problems []string
	for _, r := range rows {
		problems = append(problems, reviewRowProblem(r, columns, candidates, observed, exists, seen)...)
	}
	return problems
}

// reviewRowProblem reads one review row. Its candidate must already be a row in Security candidates, because a
// review of nothing is not a review; its outcome must be one of the three words, and each word owes a different
// decisive cell.
func reviewRowProblem(r row, columns map[string]int, candidates, observed, exists map[string]bool, seen map[string]int) []string {
	value := func(name string) string { return cell(r.cells, columns[name]) }
	id := value("id")
	var problems []string
	if first, exists := seen[id]; exists {
		problems = append(problems, fmt.Sprintf("line %d: candidate review %s repeats line %d", r.line, quote(id), first))
	} else {
		seen[id] = r.line
	}
	require := func(name string) {
		if placeholder.MatchString(value(name)) {
			problems = append(problems, fmt.Sprintf("line %d: candidate review %s requires %s", r.line, quote(id), name))
		}
	}
	candidate := strings.Trim(value("candidate id"), "`")
	if placeholder.MatchString(candidate) {
		problems = append(problems, fmt.Sprintf("line %d: candidate review %s requires a candidate id", r.line, quote(id)))
	} else if !candidates[candidate] {
		problems = append(problems, fmt.Sprintf("line %d: candidate review %s cites candidate %s, which is not a row in Security candidates", r.line, quote(id), quote(candidate)))
	}
	require("reviewer")
	if !strings.EqualFold(strings.TrimSpace(value("independence attestation")), independenceAttested) {
		problems = append(problems, fmt.Sprintf("line %d: candidate review %s requires an explicit independence attestation (the literal %q, recording that the reviewer did not produce the candidate)", r.line, quote(id), independenceAttested))
	}
	require("refutation attempt")
	require("effective controls")

	outcome := strings.ToLower(strings.TrimSpace(value("outcome")))
	if !securityReviewOutcomes[outcome] {
		return append(problems, fmt.Sprintf("line %d: candidate review %s outcome must be confirmed, rejected, or blocked", r.line, quote(id)))
	}
	evidence := strings.Trim(value("decisive evidence id"), "`")
	switch outcome {
	case "confirmed", "rejected":
		if placeholder.MatchString(evidence) {
			problems = append(problems, fmt.Sprintf("line %d: candidate review %s is %s but cites no decisive observed Evidence ledger row", r.line, quote(id), outcome))
		} else if !exists[evidence] {
			problems = append(problems, fmt.Sprintf("line %d: candidate review %s cites decisive evidence %s, which is not a row in the Evidence ledger", r.line, quote(id), quote(evidence)))
		} else if !observed[evidence] {
			problems = append(problems, fmt.Sprintf("line %d: candidate review %s requires decisive evidence %s with executed and observed details labelled observado", r.line, quote(id), quote(evidence)))
		}
	case "blocked":
		if placeholder.MatchString(value("blocker")) {
			problems = append(problems, fmt.Sprintf("line %d: candidate review %s blocked requires a specific blocker", r.line, quote(id)))
		}
		if !placeholder.MatchString(evidence) && !exists[evidence] {
			problems = append(problems, fmt.Sprintf("line %d: candidate review %s cites decisive evidence %s, which is not a row in the Evidence ledger", r.line, quote(id), quote(evidence)))
		}
	}
	return problems
}

// candidateIDs is the set of candidate rows a review may name. It reads the same fence-aware region the
// candidate contract reads, so a documentation example can never lend an id to a review.
func candidateIDs(lines []string) map[string]bool {
	ids := map[string]bool{}
	heading, end, _ := securityCandidateRegion(lines)
	if heading < 0 {
		return ids
	}
	table := scanTable(lines, heading+1, end)
	if table.header == nil {
		return ids
	}
	idColumn := exactColumn(table.header, "id")
	if idColumn < 0 {
		idColumn = 0
	}
	for _, r := range table.rows {
		if id := strings.TrimSpace(cell(r.cells, idColumn)); id != "" {
			ids[id] = true
		}
	}
	return ids
}

// securityReviewsRegion ignores documentation fences and counts every actual section, so a repeated heading is
// reported instead of silently shadowing the first one.
func securityReviewsRegion(lines []string) (heading, end, count int) {
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
		if strings.EqualFold(strings.TrimSpace(t[3:]), securityReviewsName) {
			count++
			if heading < 0 {
				heading = i
			}
		}
	}
	return
}

// reviewTableShapeProblems reads the region once for the shapes scanTable cannot report: a second table that
// starts legitimately under a subheading, and a populated row whose id is a placeholder — which the row reader
// skips, so without this pass the row would vanish from the count entirely.
func reviewTableShapeProblems(lines []string, start, end int) []string {
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
			problems = append(problems, fmt.Sprintf("line %d: Candidate reviews has more than one table", i+1))
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
				problems = append(problems, fmt.Sprintf("line %d: populated candidate review requires an id", i+1))
				break
			}
		}
	}
	return problems
}
