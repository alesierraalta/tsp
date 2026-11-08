package plan

import (
	"fmt"
	"strings"
)

// maskRequest keeps request text as data without changing its line numbers.
func maskRequest(doc string) string {
	lines := strings.Split(doc, "\n")
	inRequest, fence := false, ""
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if fence != "" {
			if strings.HasPrefix(t, fence) {
				fence = ""
			} else {
				lines[i] = ""
			}
			continue
		}
		if strings.EqualFold(t, "## Request") {
			inRequest = true
			continue
		}
		if inRequest {
			if marker := fenceMarker(t); marker != "" {
				fence = marker
				continue
			}
			if strings.HasPrefix(t, "## ") {
				inRequest = false
			}
		}
	}
	return strings.Join(lines, "\n")
}

func requestSources(lines []string) map[string]bool {
	ids := map[string]bool{}
	for _, name := range []string{"Findings", "Evidence ledger"} {
		for _, r := range scanSection(lines, name).rows {
			ids[cell(r.cells, 0)] = true
		}
	}
	return ids
}

func requestProblems(lines []string) []string {
	request, _ := sectionRegion(lines, "Request")
	heading, _ := sectionRegion(lines, "Request criteria")
	if heading < 0 {
		return nil
	}
	var problems []string
	if request < 0 || request > heading {
		problems = append(problems, fmt.Sprintf("line %d: Request criteria has no Request above it", heading+1))
	}
	scan := scanSection(lines, "Request criteria")
	_, end := sectionRegion(lines, "Request criteria")
	if scan.header != nil {
		for i := scan.head; i < end; i++ {
			t := strings.TrimSpace(lines[i])
			if !strings.HasPrefix(t, "|") {
				if t != "" {
					break
				}
				continue
			}
			cells := split(t)
			if isSeparator(cells) || !placeholder.MatchString(cell(cells, 0)) {
				continue
			}
			for _, value := range cells[1:] {
				if !placeholder.MatchString(value) {
					problems = append(problems, fmt.Sprintf("line %d: criterion [empty] needs an Id", i+1))
					break
				}
			}
		}
	}
	ids, sources := map[string]int{}, requestSources(lines)
	for _, r := range scan.rows {
		id, criterion, disposition, source, reason := cell(r.cells, 0), cell(r.cells, 1), strings.ToLower(cell(r.cells, 2)), strings.Trim(cell(r.cells, 3), "`"), cell(r.cells, 4)
		prefix := fmt.Sprintf("line %d: criterion %s", r.line, quote(id))
		if placeholder.MatchString(id) || placeholder.MatchString(criterion) {
			problems = append(problems, prefix+" needs an Id and Criterion")
		}
		if first, ok := ids[id]; ok {
			problems = append(problems, fmt.Sprintf("%s repeats the id on line %d", prefix, first))
		}
		ids[id] = r.line
		switch disposition {
		case "covered":
			if !sources[source] || placeholder.MatchString(source) {
				problems = append(problems, prefix+" cites no existing Finding or Evidence row")
			}
		case "blocked", "na":
			if placeholder.MatchString(reason) {
				problems = append(problems, prefix+" needs a reason")
			}
		default:
			problems = append(problems, prefix+" disposition must be covered, blocked, or na")
		}
	}
	return problems
}

// RequestNotice reports a request that has no criteria table without rejecting it.
func RequestNotice(doc string) string {
	lines := strings.Split(maskRequest(doc), "\n")
	request, _ := sectionRegion(lines, "Request")
	if request >= 0 && scanSection(lines, "Request criteria").header == nil {
		return "request has no criteria table; acceptance criteria are unproven"
	}
	return ""
}
