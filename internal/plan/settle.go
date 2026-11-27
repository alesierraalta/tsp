package plan

import (
	"fmt"
	"os"
	"strings"
)

// SettleEdit is the change `plan settle` makes to one existing Findings row: the status the row now holds, and the
// witness or reason that status owes. A field the caller leaves empty keeps the row's own cell, so settling a
// repaired defect does not erase the evidence it was recorded with.
type SettleEdit struct {
	ID        string
	Status    string
	Test      string
	Reason    string
	VerdictBy string
}

// Settle rewrites one Findings row in place after its verdict changed — the repair that landed, the witness that
// now passes, the defect that will not be fixed — and returns the 1-based line it rewrote.
//
// It exists because add-finding is append-only on purpose: a second row for one defect would split its history,
// and a hand-edited row is a verdict nobody checked. Only a writer that names the row it settles, validates the
// document it produces and holds the plan's own lock may move a verdict.
//
// Every refusal leaves the file byte-identical: the candidate is held to CheckDocument — the rules `plan check`
// runs — before a single byte moves. Whether a settled witness really behaves as its status claims is
// `plan check --pins`'s question, not this writer's: a weak test cannot be detected by rewriting a table.
func Settle(path string, edit SettleEdit) (int, error) {
	target := canonicalPath(path)
	lock, err := LockPlanWithin(target, lockWait)
	if err != nil {
		return 0, err
	}
	defer UnlockPlan(lock)
	return settle(target, edit)
}

// settle is the transaction Settle holds the plan's lock across: one read, one candidate row, one write.
func settle(path string, edit SettleEdit) (int, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	lines := strings.Split(string(raw), "\n")
	table, err := findingsTable(lines)
	if err != nil {
		return 0, err
	}
	finding, line, err := settledRow(table, edit)
	if err != nil {
		return 0, err
	}
	if err := finding.checkValues(false); err != nil {
		return 0, err
	}
	row, err := finding.renderOver(cellsOfRow(lines[line-1], table.header), table.header, finding.Status, false)
	if err != nil {
		return 0, err
	}
	candidate := strings.Join(replaceLine(lines, line, row), "\n")
	if problems := CheckDocument(candidate); len(problems) > 0 {
		return 0, fmt.Errorf("the settled row would not pass plan check: %s; the plan is unchanged", strings.Join(problems, "; "))
	}
	if err := writePlan(path, candidate); err != nil {
		return 0, err
	}
	return line, nil
}

// settledRow reads the one row the edit names, applies the edit and checks the obligations the target status
// carries. The witness and the reason keep the row's own value unless the caller supplies one: a settled row
// already names the test that holds its verdict, and the decision that keeps a defect is the decision being made
// now, which is why a reason is never inherited.
func settledRow(table tableScan, edit SettleEdit) (Finding, int, error) {
	if strings.TrimSpace(edit.ID) == "" {
		return Finding{}, 0, usagef("--id is required: the Findings row this settle rewrites")
	}
	status, err := settleStatus(edit.Status)
	if err != nil {
		return Finding{}, 0, err
	}
	if err := unfitValues(table.header, edit); err != nil {
		return Finding{}, 0, err
	}
	matches := rowsWithID(table, edit.ID)
	switch len(matches) {
	case 0:
		return Finding{}, 0, fmt.Errorf("the plan holds no finding %s, so there is nothing to settle", quote(edit.ID))
	case 1:
	default:
		return Finding{}, 0, fmt.Errorf("the plan holds finding %s on lines %d and %d, so settle cannot tell which row to rewrite",
			quote(edit.ID), matches[0].line, matches[1].line)
	}
	finding := findingFromRow(matches[0], table.header)
	if strings.TrimSpace(edit.Test) != "" {
		finding.Test = edit.Test
	}
	if strings.TrimSpace(edit.Reason) != "" {
		finding.Reason = edit.Reason
	}
	if strings.TrimSpace(edit.VerdictBy) != "" {
		finding.VerdictBy = edit.VerdictBy
	}
	finding.Status = status
	if settledStatus.MatchString(status) && placeholder.MatchString(strings.TrimSpace(finding.Test)) {
		return Finding{}, 0, usagef("--status %s is settled, so the row owes the witness that holds it: pass --test '<suite path> :: <test name>'", status)
	}
	if status == "rejected" || status == "wontfix" {
		if strings.TrimSpace(edit.Reason) == "" {
			return Finding{}, 0, usagef("--status %s owes the reason the defect stays: pass --reason '<why>'", status)
		}
		if placeholder.MatchString(strings.TrimSpace(finding.Reason)) {
			return Finding{}, 0, usagef("--reason is a placeholder, so it says nothing about why the defect stays")
		}
	}
	return finding, matches[0].line, nil
}

// settleStatus reads --status against the closed vocabulary the checker enforces, so the writer and the checker
// cannot drift: a value outside it is refused with the list rather than written, and an empty one is not read as
// `open` — settling is a decision, and a decision nobody named is not one.
func settleStatus(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", usagef("--status is required: one of %s", FindingsStatusList)
	}
	status, breach := findingStatus(raw)
	if breach != "" {
		return "", usagef("--status %s is not one of: %s", quote(raw), FindingsStatusList)
	}
	return status, nil
}

// unfitValues refuses a value the caller supplied where the table has no column to carry it: accepting the flag
// and dropping the text would record a verdict whose witness or reason is nowhere in the plan.
func unfitValues(header []string, edit SettleEdit) error {
	for _, v := range []struct{ flag, column, value string }{
		{"--test", "pinning test", edit.Test},
		{"--reason", "reason", edit.Reason},
		{"--verdict-by", "verdict by", edit.VerdictBy},
	} {
		if strings.TrimSpace(v.value) != "" && columnIndex(header, v.column) < 0 {
			return usagef("%s cannot land: the Findings header has no %s column", v.flag, v.column)
		}
	}
	return nil
}

// rowsWithID returns every row whose Id cell names id. A cell may wrap the id in backticks, and the other readers
// -- `plan check --pins` and `--replay` -- strip them before matching, so this reads an id the same way. The
// checker refuses a repeated id, so a table that holds two is already broken; settle refuses to pick one and leave
// the other claiming the same defect.
func rowsWithID(table tableScan, id string) []row {
	var matches []row
	at := idColumn(table.header)
	want := strings.Trim(strings.TrimSpace(id), "`")
	for _, r := range table.rows {
		if strings.EqualFold(strings.Trim(cell(r.cells, at), "`"), want) {
			matches = append(matches, r)
		}
	}
	return matches
}

// findingFromRow reads one Findings row into the value the writer renders, by column name: a row that moved does
// not move the reading with it.
func findingFromRow(r row, header []string) Finding {
	cols := columnsOf(header)
	return Finding{
		ID:           cell(r.cells, idColumn(header)),
		Location:     cell(r.cells, cols.find),
		Severity:     cell(r.cells, columnIndex(header, "severity")),
		DataSafe:     cell(r.cells, columnIndex(header, "data safe")),
		Evidence:     cell(r.cells, cols.evidence),
		Test:         cell(r.cells, cols.pin),
		Status:       cell(r.cells, cols.status),
		VerdictBy:    cell(r.cells, columnIndex(header, "verdict by")),
		Reason:       cell(r.cells, columnIndex(header, "reason")),
		Fingerprint:  cell(r.cells, cols.fingerprint),
		Causality:    cell(r.cells, cols.causality),
		Reproduction: cell(r.cells, columnIndex(header, "reproduction")),
	}
}

// replaceLine puts row where the 1-based line at sits, leaving every other line where it was.
func replaceLine(lines []string, at int, row string) []string {
	out := append([]string{}, lines...)
	if at >= 1 && at <= len(out) {
		out[at-1] = row
	}
	return out
}

// cellsOfRow reads the row about to be rewritten into the cells the renderer overwrites: a column this writer does
// not name keeps the cell the row already carries. A row that does not line up with its header is rebuilt empty,
// and the checker refuses the candidate rather than a half-read row landing.
func cellsOfRow(line string, header []string) []string {
	cells := split(line)
	if len(cells) != len(header) {
		return make([]string, len(header))
	}
	return cells
}
