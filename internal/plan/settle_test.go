package plan

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// settlePlan is a plan whose rows cover the three states a settle has to tell apart: two confirmed defects that
// name their witness, and one open row that names none yet. One plan for every case keeps each test varying
// exactly one thing.
func settlePlan(t *testing.T) string {
	t.Helper()
	p := write(t, t.TempDir(), "plan.md", addPlan())
	unwitnessed := openFinding()
	unwitnessed.ID = "F3"
	for _, f := range []Finding{
		confirmedFinding("F1", "src/a.test.js :: a"),
		confirmedFinding("F2", "src/b.test.js :: b"),
		unwitnessed,
	} {
		if _, err := AddFinding(p, f); err != nil {
			t.Fatalf("AddFinding %s: %v", f.ID, err)
		}
	}
	return p
}

func confirmedFinding(id, test string) Finding {
	f := openFinding()
	f.ID, f.Status, f.Test = id, "confirmed", test
	return f
}

func readPlan(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// The repair landed and the witness it named now passes: the row stops claiming the defect is still there, and
// every other byte of the plan stays where the reader left it. A settle that rewrote the table would erase the
// rows it did not decide.
func TestSettleTurnsARepairedRowFixedAndLeavesTheRestAlone(t *testing.T) {
	p := settlePlan(t)
	before := readPlan(t, p)

	line, err := Settle(p, SettleEdit{ID: "F1", Status: "fixed"})
	if err != nil {
		t.Fatalf("Settle: %v", err)
	}
	after := readPlan(t, p)
	if problems := CheckDocument(after); len(problems) != 0 {
		t.Fatalf("the row settle wrote is rejected by its own checker: %v", problems)
	}

	oldLines, newLines := strings.Split(before, "\n"), strings.Split(after, "\n")
	if len(oldLines) != len(newLines) {
		t.Fatalf("settle changed the line count: %d -> %d", len(oldLines), len(newLines))
	}
	changed := 0
	for i := range oldLines {
		if oldLines[i] == newLines[i] {
			continue
		}
		changed++
		if i+1 != line {
			t.Fatalf("settle rewrote line %d, not the reported line %d", i+1, line)
		}
	}
	if changed != 1 {
		t.Fatalf("settle rewrote %d lines, want 1", changed)
	}
	row := newLines[line-1]
	for _, want := range []string{"| fixed |", "src/a.test.js :: a"} {
		if !strings.Contains(row, want) {
			t.Fatalf("the settled row lost %q: %s", want, row)
		}
	}
	if !strings.HasPrefix(row, "| F1 |") {
		t.Fatalf("the reported line %d is not F1: %s", line, row)
	}
}

// Every refusal leaves the file byte-identical: a settle that half-writes moves a verdict nobody asked it to
// move, and the reader cannot tell it happened.
func TestSettleRefusesWithoutMovingTheFile(t *testing.T) {
	cases := []struct {
		name  string
		edit  SettleEdit
		want  string
		usage bool // the value refusals the CLI maps to exit 2
	}{
		{"an id the plan does not hold", SettleEdit{ID: "F9", Status: "fixed"}, "F9", false},
		{"a status outside the vocabulary", SettleEdit{ID: "F1", Status: "resolved"}, "open, confirmed, fixed, gap-closed, rejected, wontfix", true},
		{"a settled status on a row that names no witness", SettleEdit{ID: "F3", Status: "fixed"}, "test", true},
		{"a rejection with no reason", SettleEdit{ID: "F1", Status: "rejected"}, "reason", true},
		{"no row named at all", SettleEdit{Status: "fixed"}, "--id", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := settlePlan(t)
			before := readPlan(t, p)
			_, err := Settle(p, tc.edit)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Settle = %v, want a refusal naming %q", err, tc.want)
			}
			if got := errors.Is(err, ErrUsage); got != tc.usage {
				t.Fatalf("ErrUsage = %v, want %v: %v", got, tc.usage, err)
			}
			if after := readPlan(t, p); after != before {
				t.Fatalf("a refusal moved the plan:\n--- before\n%s\n--- after\n%s", before, after)
			}
		})
	}
}

// A row recorded without a witness gets one at settle time: the repair is only settled when a test proves it.
func TestSettleTakesTheWitnessTheRepairLanded(t *testing.T) {
	p := settlePlan(t)
	line, err := Settle(p, SettleEdit{ID: "F3", Status: "fixed", Test: "src/c.test.js :: c"})
	if err != nil {
		t.Fatalf("Settle: %v", err)
	}
	row := strings.Split(readPlan(t, p), "\n")[line-1]
	for _, want := range []string{"| fixed |", "src/c.test.js :: c"} {
		if !strings.Contains(row, want) {
			t.Fatalf("the settled row lacks %q: %s", want, row)
		}
	}
	if problems := CheckDocument(readPlan(t, p)); len(problems) != 0 {
		t.Fatalf("the settled plan is rejected by its own checker: %v", problems)
	}
}

// The verdict that a defect will not be repaired owes the reason it stays, and the reason is the one this
// decision is made with, never the one the row was opened with.
func TestSettleRejectedCarriesTheReasonItWasDecidedWith(t *testing.T) {
	p := settlePlan(t)
	line, err := Settle(p, SettleEdit{ID: "F1", Status: "wontfix", Reason: "the contract allows the loss"})
	if err != nil {
		t.Fatalf("Settle: %v", err)
	}
	row := strings.Split(readPlan(t, p), "\n")[line-1]
	for _, want := range []string{"| wontfix |", "the contract allows the loss"} {
		if !strings.Contains(row, want) {
			t.Fatalf("the settled row lacks %q: %s", want, row)
		}
	}
}

// Two rows claiming one id make the row a verdict belongs to ambiguous, so settle refuses instead of rewriting
// whichever came first.
func TestSettleRefusesAnAmbiguousID(t *testing.T) {
	doc := header +
		"| F1 | src/a.js:5 | M | yes | E1 | a.js :: a | confirmed | me | - | - |\n" +
		"| F1 | src/b.js:5 | M | yes | E1 | b.js :: b | confirmed | me | - | - |\n" +
		ledger + "| E1 | it drops the comma | `node --test` | `a,b` | 3 fields | reverted -> red | same input | observado |\n"
	p := write(t, t.TempDir(), "plan.md", doc)
	_, err := Settle(p, SettleEdit{ID: "F1", Status: "fixed"})
	if err == nil || !strings.Contains(err.Error(), "F1") {
		t.Fatalf("Settle = %v, want a refusal naming F1", err)
	}
	if after := readPlan(t, p); after != doc {
		t.Fatal("the refusal moved the plan")
	}
}

// A plan written before a column existed is still a plan the checker accepts, and the rows in it already live in
// that table's columns. Settling one of those rows must not demand a value the table has no column to carry: the
// row never had a Data safe cell, so writing one would invent it, and refusing leaves the command unusable on a
// plan `plan check` calls well formed.
const legacyFindings = "## Findings\n\n| Id | Finding | Severity | Evidence id | Pinning test | Status | Verdict by / date | Reason | Fingerprint |\n|---|---|---|---|---|---|---|---|---|\n"

func legacyPlan() string {
	return legacyFindings +
		"| F1 | src/a.js:5 | data loss | E1 | src/a.test.js :: a | confirmed | me / 2026-09-10 | not pinned | - |\n" +
		ledger + "| E1 | it drops the comma | `node --test` | `a,b` | 3 fields | reverted -> red | same input | observado |\n"
}

func TestSettleRewritesAPlanThatPredatesAColumn(t *testing.T) {
	doc := legacyPlan()
	if problems := CheckDocument(doc); len(problems) != 0 {
		t.Fatalf("the fixture must be a plan the checker accepts: %v", problems)
	}
	p := write(t, t.TempDir(), "plan.md", doc)
	line, err := Settle(p, SettleEdit{ID: "F1", Status: "fixed"})
	if err != nil {
		t.Fatalf("a plan the checker accepts must be settlable: %v", err)
	}
	row := strings.Split(readPlan(t, p), "\n")[line-1]
	for _, want := range []string{"| fixed |", "src/a.test.js :: a", "| data loss |"} {
		if !strings.Contains(row, want) {
			t.Fatalf("the settled row lacks %q: %s", want, row)
		}
	}
	if cells := len(split(row)); cells != 9 {
		t.Fatalf("settle changed the row's column count to %d, want the header's 9: %s", cells, row)
	}
}

// The tolerance is settle's alone: a row add-finding creates is told every value and owes each one a column, so a
// table that cannot carry a value still refuses the row rather than dropping what the caller supplied.
func TestAddFindingStillRefusesAPlanWithoutTheDataSafeColumn(t *testing.T) {
	p := write(t, t.TempDir(), "plan.md", legacyPlan())
	if _, err := AddFinding(p, openFinding()); err == nil || !strings.Contains(err.Error(), "Data safe") {
		t.Fatalf("AddFinding = %v, want a refusal naming the missing column", err)
	}
}

// A value the caller names has to land. A header without the column cannot carry it, and accepting the flag while
// dropping the text would record a verdict whose reason is nowhere in the plan.
func TestSettleRefusesAValueTheHeaderCannotCarry(t *testing.T) {
	const noReason = "## Findings\n\n| Id | Finding | Severity | Data safe? | Evidence id | Pinning test | Status | Verdict by / date | Fingerprint |\n|---|---|---|---|---|---|---|---|---|\n" +
		"| F1 | src/a.js:5 | data loss | yes | E1 | src/a.test.js :: a | confirmed | me / 2026-09-10 | - |\n" +
		ledger + "| E1 | it drops the comma | `node --test` | `a,b` | 3 fields | reverted -> red | same input | observado |\n"
	if problems := CheckDocument(noReason); len(problems) != 0 {
		t.Fatalf("the fixture must be a plan the checker accepts: %v", problems)
	}
	p := write(t, t.TempDir(), "plan.md", noReason)
	_, err := Settle(p, SettleEdit{ID: "F1", Status: "wontfix", Reason: "the contract allows the loss"})
	if err == nil || !strings.Contains(err.Error(), "--reason") {
		t.Fatalf("Settle = %v, want a refusal naming the flag and the column it cannot land in", err)
	}
	if !errors.Is(err, ErrUsage) {
		t.Fatalf("a value that cannot land is the invocation's error, exit 2: %v", err)
	}
	if after := readPlan(t, p); after != noReason {
		t.Fatal("the refusal moved the plan")
	}
}

// A cell may wrap an id in backticks: `plan check` reads the row as well formed, and `--pins` and `--replay` strip
// them before matching. Settle reads ids the same way, or a plan the checker accepts cannot be settled by the id
// every other reader reports.
func TestSettleFindsARowWhoseIdIsBackticked(t *testing.T) {
	doc := strings.Replace(legacyPlan(), "| F1 |", "| `F1` |", 1)
	if problems := CheckDocument(doc); len(problems) != 0 {
		t.Fatalf("the fixture must be a plan the checker accepts: %v", problems)
	}
	p := write(t, t.TempDir(), "plan.md", doc)
	line, err := Settle(p, SettleEdit{ID: "F1", Status: "fixed"})
	if err != nil {
		t.Fatalf("Settle: %v", err)
	}
	row := strings.Split(readPlan(t, p), "\n")[line-1]
	if !strings.Contains(row, "| fixed |") || !strings.Contains(row, "| `F1` |") {
		t.Fatalf("the settled row lost its id or its status: %s", row)
	}
}
