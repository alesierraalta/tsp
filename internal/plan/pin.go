package plan

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alesierraalta/tsp/internal/citation"
)

// PinningSection is the optional section a chained run keeps: one row per finding another reviewer produced, and
// what this run did about it. It is separate from Findings, which holds the defects this run found itself.
const PinningSection = "Pinning"

// PinningHeader is the table every Pinning section carries.
const PinningHeader = "| Id | Finding (path:line, one line) | Claim (one line) | Status | Test (suite path :: test name) | Reason |\n|---|---|---|---|---|---|\n"

// pinningStatuses is the closed vocabulary of what a run did with a finding another reviewer produced.
var pinningStatuses = map[string]bool{"open": true, "pinned": true, "blocked": true, "rejected": true}

// Pin is one row of the Pinning section.
type Pin struct {
	ID       string
	Location string
	Claim    string
	Status   string
	Test     string
	Reason   string
}

// PinRow renders the pin as one Markdown table row.
func (p Pin) PinRow() string {
	return fmt.Sprintf("| %s | %s | %s | %s | %s | %s |\n", pinCell(p.ID), pinCell(p.Location), pinCell(p.Claim), pinCell(p.Status), pinCell(p.Test), pinCell(p.Reason))
}

// pinCell keeps one value on one table row and in one column.
func pinCell(s string) string {
	s = strings.ReplaceAll(s, "|", "/")
	s = strings.ReplaceAll(s, "\n", " ")
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return strings.TrimSpace(s)
}

// AddPins appends the pins the plan does not hold yet, in order, and reports how many rows it added. A pin whose
// location and claim are already in the section is not added twice: running the same chain twice must not double
// the list. The file is left byte-identical when nothing is added or the plan cannot be read.
func AddPins(planPath string, pins []Pin) (int, error) {
	data, err := os.ReadFile(planPath)
	if err != nil {
		return 0, err
	}
	doc := string(data)
	_, rows := Table(doc, PinningSection)
	type key struct{ location, claim string }
	seen := map[key]bool{}
	for _, r := range rows {
		if len(r) > 2 {
			seen[key{strings.TrimSpace(r[1]), strings.TrimSpace(r[2])}] = true
		}
	}
	var added strings.Builder
	n := 0
	for _, p := range pins {
		if seen[key{p.Location, p.Claim}] {
			continue
		}
		seen[key{p.Location, p.Claim}] = true
		added.WriteString(p.PinRow())
		n++
	}
	if n == 0 {
		return 0, nil
	}
	if !strings.Contains(doc, "## "+PinningSection) {
		if !strings.HasSuffix(doc, "\n") {
			doc += "\n"
		}
		doc += "\n## " + PinningSection + "\n\n" + PinningHeader + "\n"
		// The section we just wrote ends with its header; the new rows belong under it, before any later section.
	}
	out := insertRows(doc, PinningSection, added.String())
	// Only this section is held to its own rules here: adding a pin must not depend on the rest of the plan, which
	// `plan check` audits on its own.
	if problems := pinningProblems(out, ""); len(problems) > 0 {
		return 0, fmt.Errorf("the pins would make the section invalid: %s", strings.Join(problems, "; "))
	}
	if err := os.WriteFile(planPath, []byte(out), 0o644); err != nil {
		return 0, err
	}
	return n, nil
}

// insertRows puts the rows under the section's table, before whatever follows it.
func insertRows(doc, section, rows string) string {
	lines := strings.Split(doc, "\n")
	heading := -1
	for i, line := range lines {
		if strings.EqualFold(strings.TrimSpace(line), "## "+section) {
			heading = i
		}
	}
	if heading < 0 {
		return doc
	}
	end := len(lines)
	for i := heading + 1; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "## ") {
			end = i
			break
		}
	}
	at := end
	// After the separator row when the section already has a table, at the end of the section otherwise.
	for i := heading + 1; i < end; i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "|---") {
			at = i + 1
			break
		}
	}
	out := append([]string{}, lines[:at]...)
	out = append(out, strings.Split(strings.TrimRight(rows, "\n"), "\n")...)
	out = append(out, lines[at:]...)
	return strings.Join(out, "\n")
}

// SetPinStatus rewrites one pin's status, and its test or reason, leaving every other byte of the plan alone.
func SetPinStatus(planPath, id, status, test, reason string) error {
	data, err := os.ReadFile(planPath)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	inSection := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			inSection = strings.EqualFold(trimmed, "## "+PinningSection)
			continue
		}
		if !inSection || !strings.HasPrefix(trimmed, "|") {
			continue
		}
		cells := strings.Split(strings.Trim(trimmed, "|"), "|")
		if len(cells) < 6 || strings.Trim(strings.TrimSpace(cells[0]), "`") != id {
			continue
		}
		// Keep the columns the caller did not set.
		if test == "" {
			test = strings.TrimSpace(cells[4])
		}
		if reason == "" {
			reason = strings.TrimSpace(cells[5])
		}
		lines[i] = Pin{ID: id, Location: strings.TrimSpace(cells[1]), Claim: strings.TrimSpace(cells[2]), Status: status, Test: test, Reason: reason}.PinRow()
		return os.WriteFile(planPath, []byte(strings.Join(lines, "\n")), 0o644)
	}
	return fmt.Errorf("the plan holds no pinning row %s", quote(id))
}

// pinningProblems holds the section to its own rules: a closed status vocabulary, a recorded test for a pin and a
// reason for the two outcomes that say why nothing was pinned.
func pinningProblems(doc, dir string) []string {
	if !strings.Contains(doc, "## "+PinningSection) {
		return nil
	}
	_, rows := Table(doc, PinningSection)
	seen := map[string]bool{}
	var problems []string
	for i, r := range rows {
		line := i + 1
		if len(r) < 6 {
			continue
		}
		id := strings.Trim(strings.TrimSpace(r[0]), "`")
		if id == "" || id == "-" {
			problems = append(problems, fmt.Sprintf("pinning row %d has no id", line))
			continue
		}
		if seen[id] {
			problems = append(problems, fmt.Sprintf("pinning row %s repeats an id", quote(id)))
		}
		seen[id] = true
		status := strings.ToLower(strings.TrimSpace(r[3]))
		if !pinningStatuses[status] {
			problems = append(problems, fmt.Sprintf("pinning row %s status must be open, pinned, blocked, or rejected, not %s", quote(id), quote(r[3])))
			continue
		}
		if len(citation.Extract(r[1])) == 0 {
			problems = append(problems, fmt.Sprintf("pinning row %s cites no path:line, so the finding cannot be located", quote(id)))
		}
		switch status {
		case "pinned":
			file := strings.TrimSpace(strings.Split(r[4], "::")[0])
			file = strings.Trim(strings.TrimSpace(file), "`")
			if file == "-" || file == "" {
				problems = append(problems, fmt.Sprintf("pinning row %s is pinned but names no test", quote(id)))
			} else if dir != "" {
				if _, err := os.Stat(filepath.Join(dir, file)); err != nil {
					problems = append(problems, fmt.Sprintf("pinning row %s names the test %s, which the plan's directory does not hold", quote(id), quote(file)))
				}
			}
		case "blocked", "rejected":
			if strings.TrimSpace(r[5]) == "-" || strings.TrimSpace(r[5]) == "" {
				problems = append(problems, fmt.Sprintf("pinning row %s is %s but gives no reason", quote(id), status))
			}
		}
	}
	return problems
}
