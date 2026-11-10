package bench

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	plancheck "github.com/alesierraalta/tsp/internal/plan"
)

// How a Pinning row states what the chain did with a finding, as the plan module spells it: a finding is pinned
// with a test, blocked when the pin could not be attempted, and rejected when the claim does not hold — the spelling
// `tsp pin --refute` writes, read here as the refutation it is.
const (
	pinStatusPinned   = "pinned"
	pinStatusBlocked  = "blocked"
	pinStatusRejected = "rejected"
	pinStatusOpen     = "open"
)

// The Pinning table's columns, in the order plan.PinningHeader declares them.
const (
	pinColID = iota
	pinColLocation
	pinColClaim
	pinColStatus
	pinColTest
	pinColReason
)

// PinPrompt is the whole instruction a chain arm's agent receives: another review's findings, and what to do with
// each one. Nothing of the case's request reaches this arm, because the review already made the claim it is asked
// to pin (S1, S2).
func PinPrompt(findingsPath string) string {
	return fmt.Sprintf(`Read the review's findings at %s. Every finding cites a path:line and a claim. For each one, write one test that fails on this code and passes on the fixed code, and record it with %s. A finding you cannot pin is recorded with %s; a finding whose claim does not hold is recorded with %s. Nothing else records a pin, so a finding you leave unrecorded is a finding this run did not answer. Do not change any source file.`,
		findingsPath,
		pinCommand("tsp pin --record <id> --test '<suite path> :: <test name>'"),
		pinCommand("tsp pin --block <id> --reason '<why the pin could not be attempted>'"),
		pinCommand("tsp pin --refute <id> --reason '<why the claim does not hold>'"))
}

// pinCommand renders one of the commands the prompt names the way the skill's own instructions render one.
func pinCommand(cmd string) string { return "`" + cmd + "`" }

// PinChainResult is the chain arm's own reading of one run: what its plan says it did with every finding the review
// handed it (S4). It is read beside a run's detection and never folded into it.
type PinChainResult struct {
	Pinned  int `json:"pinned"`
	Refuted int `json:"refuted"`
	Blocked int `json:"blocked"`
	Open    int `json:"open"`
	// Defects lists, in key order, the case defects the pinned findings pinned: the count comparable to the TSP
	// arm's own caught on the same case.
	Defects []string     `json:"defects,omitempty"`
	Rows    []PinOutcome `json:"rows,omitempty"`
}

// PinOutcome is one row of the Pinning section: the finding as the review worded it, and what this run did about it.
type PinOutcome struct {
	ID       string `json:"id"`
	Location string `json:"location"`
	Claim    string `json:"claim,omitempty"`
	Status   string `json:"status"`
	Test     string `json:"test,omitempty"`
}

// ScorePinning reads the plan a workspace delivered and counts what the chain did with the findings it was handed.
// The plan is resolved the way every reading resolves one, so the section scored is the section plan check audits.
// Only a row the plan records counts (S3): a pin nobody recorded is a pin this bench never reports. A plan or a
// Pinning section that is absent recorded nothing: an empty result, never an error.
func ScorePinning(ws string) PinChainResult {
	rows := pinnedRows(ws)
	chain := PinChainResult{Rows: rows}
	for _, row := range rows {
		switch row.Status {
		case pinStatusPinned:
			chain.Pinned++
		case pinStatusBlocked:
			chain.Blocked++
		case pinStatusRejected:
			chain.Refuted++
		default:
			// Open, and a row that names no decided status at all: both still wait for an answer.
			chain.Open++
		}
	}
	return chain
}

// PinnedDefects maps each pinned finding to the case defect it pins, by location, through the very matcher a
// review's findings are scored with: a pinned row that cites a defect's file at its line, or names it, is that
// defect pinned. Only a pinned row counts; a blocked or refuted finding pinned nothing.
func PinnedDefects(ws string, key Key) map[string]bool {
	pinned := map[string]bool{}
	for _, row := range pinnedRows(ws) {
		if row.Status != pinStatusPinned {
			continue
		}
		ids, matched := claimed(row.Location+" "+row.Claim, "", key.Defects)
		if !matched {
			continue
		}
		for id := range ids {
			pinned[id] = true
		}
	}
	return pinned
}

// pinnedRows reads every Pinning row the workspace's plan carries. A plan that cannot be selected, is not there, or
// holds no Pinning section yields no row: a chain that recorded nothing is a reading of zero, not a failure.
func pinnedRows(ws string) []PinOutcome {
	path, resolution := resolvePlanPath(ws)
	if resolution.refused {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(ws, path))
	if err != nil {
		return nil
	}
	_, table := plancheck.Table(string(data), plancheck.PinningSection)
	var rows []PinOutcome
	for _, row := range table {
		if len(row) <= pinColReason {
			// A row shorter than the section declares is not a row this reader can answer for.
			continue
		}
		rows = append(rows, PinOutcome{
			ID:       strings.Trim(row[pinColID], "`"),
			Location: pinCellText(row[pinColLocation]),
			Claim:    pinCellText(row[pinColClaim]),
			Status:   strings.ToLower(row[pinColStatus]),
			Test:     pinCellText(row[pinColTest]),
		})
	}
	return rows
}

// pinCellText drops the placeholder a row writes when it has nothing to say, so a claim of "-" is no claim.
func pinCellText(cell string) string {
	if placeholderRe.MatchString(strings.TrimSpace(cell)) {
		return ""
	}
	return strings.Trim(strings.TrimSpace(cell), "`")
}
