package plan

import (
	"fmt"
	"strings"
)

// securityCandidateProblems checks the optional security-specific contract without changing Findings statuses.
// It validates recorded evidence references and declarations, not whether a claim is actually true.
func securityCandidateProblems(lines []string, ledger tableScan) []string {
	ledgerRows := map[string]row{}
	for _, r := range ledger.rows {
		ledgerRows[cell(r.cells, 0)] = r
	}
	heading, end, regions := securityCandidateRegion(lines)
	if heading < 0 {
		return nil
	}
	var problems []string
	if regions > 1 {
		problems = append(problems, "Security candidates section is repeated")
	}
	table := scanTable(lines, heading+1, end)
	if table.header == nil {
		return append(problems, fmt.Sprintf("line %d: Security candidates has no table", heading+1))
	}
	problems = append(problems, securityTableShapeProblems(lines, heading+1, end)...)
	if line, message := interruptedTable("Security candidates", table); message != "" {
		problems = append(problems, fmt.Sprintf("line %d: %s", line, message))
	}
	columns := map[string]int{}
	for i, header := range table.header {
		name := strings.ToLower(strings.TrimSpace(header))
		if _, exists := columns[name]; exists {
			problems = append(problems, fmt.Sprintf("line %d: Security candidates repeats column %s", table.head, quote(name)))
		}
		columns[name] = i
	}
	for _, name := range []string{"id", "actor", "authority", "resource", "action", "boundary", "evidence id", "disposition", "observed boundary violation", "missing fact", "safe validation step", "refuting evidence id"} {
		if _, exists := columns[name]; !exists {
			problems = append(problems, fmt.Sprintf("line %d: Security candidates has no %s column", table.head, name))
		}
	}
	if len(problems) != 0 {
		return problems
	}
	seen := map[string]int{}
	for _, r := range table.rows {
		value := func(name string) string { return cell(r.cells, columns[name]) }
		id := value("id")
		if first, exists := seen[id]; exists {
			problems = append(problems, fmt.Sprintf("line %d: security candidate %s repeats line %d", r.line, quote(id), first))
		} else {
			seen[id] = r.line
		}
		require := func(name string) {
			if placeholder.MatchString(value(name)) {
				problems = append(problems, fmt.Sprintf("line %d: security candidate %s requires %s", r.line, quote(id), name))
			}
		}
		evidence := func(name string) {
			v := strings.Trim(value(name), "`")
			if placeholder.MatchString(v) {
				require(name)
			} else if record, exists := ledgerRows[v]; !exists {
				problems = append(problems, fmt.Sprintf("line %d: security candidate %s cites evidence %s, which is not a row in the Evidence ledger", r.line, quote(id), quote(v)))
			} else if !strings.EqualFold(cell(record.cells, exactColumn(ledger.header, "label")), "observado") || placeholder.MatchString(cell(record.cells, exactColumn(ledger.header, "executed"))) || placeholder.MatchString(cell(record.cells, exactColumn(ledger.header, "observed"))) {
				problems = append(problems, fmt.Sprintf("line %d: security candidate %s requires evidence %s with executed and observed details labelled observado", r.line, quote(id), quote(v)))
			}
		}
		for _, name := range []string{"actor", "authority", "resource", "action", "boundary"} {
			require(name)
		}
		switch strings.ToLower(value("disposition")) {
		case "confirmed":
			require("observed boundary violation")
			evidence("evidence id")
		case "blocked":
			require("missing fact")
			require("safe validation step")
		case "rejected":
			evidence("refuting evidence id")
		default:
			problems = append(problems, fmt.Sprintf("line %d: security candidate %s disposition must be confirmed, blocked, or rejected", r.line, quote(id)))
		}
	}
	return problems
}

// securityCandidateRegion ignores documentation fences and counts every actual section.
func securityCandidateRegion(lines []string) (heading, end, count int) {
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
		if strings.EqualFold(strings.TrimSpace(t[3:]), "Security candidates") {
			count++
			if heading < 0 {
				heading = i
			}
		}
	}
	return
}

func securityTableShapeProblems(lines []string, start, end int) []string {
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
			problems = append(problems, fmt.Sprintf("line %d: Security candidates has more than one table", i+1))
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
				problems = append(problems, fmt.Sprintf("line %d: populated security candidate requires an id", i+1))
				break
			}
		}
	}
	return problems
}

func exactColumn(header []string, name string) int {
	found := -1
	for i, h := range header {
		column := strings.ToLower(strings.TrimSpace(h))
		if column == name || (name == "label" && column == "label (`observado` / `razonado`, literal)") || (name == "executed" && column == "executed (prose)") {
			if found >= 0 {
				return -1
			}
			found = i
		}
	}
	return found
}
