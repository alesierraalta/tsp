package bench

import (
	"strings"
	"testing"
)

// noteCell returns the note column of the one row for a case, so an assertion talks about what a
// reader sees rather than about how the string happens to be assembled.
func noteCell(t *testing.T, text, name string) string {
	t.Helper()
	for _, line := range strings.Split(text, "\n") {
		if !strings.HasPrefix(line, "| "+name+" |") {
			continue
		}
		cells := strings.Split(line, "|")
		if len(cells) < 13 {
			t.Fatalf("row %q has %d cells, want 13", line, len(cells))
		}
		return strings.TrimSpace(cells[11])
	}
	t.Fatalf("no row for case %q in:\n%s", name, text)
	return ""
}

// TestNoPlanCellSaysItOnce pins the reading of a run that wrote no plan: "NO PLAN" is the whole
// statement, whether the result carries the note a scorer writes when the plan file is absent or not.
// The tag is derived from PlanFound, so repeating the same fact as a note reads like two problems
// where there is one.
func TestNoPlanCellSaysItOnce(t *testing.T) {
	bare := Aggregate{Cases: []Result{{Case: "c", Family: "time", Total: 1}}}
	noted := Aggregate{Cases: []Result{{Case: "c", Family: "time", Total: 1, Notes: []string{"no plan"}}}}

	for name, agg := range map[string]Aggregate{"plan": bare, "planless": bare} {
		if got := noteCell(t, Summary(agg), "c"); got != "NO PLAN" {
			t.Fatalf("%s summary note = %q, want %q", name, got, "NO PLAN")
		}
	}
	if got, want := noteCell(t, Summary(noted), "c"), noteCell(t, Summary(bare), "c"); got != want {
		t.Fatalf("a no-plan note changed the cell: got %q, want %q", got, want)
	}
	// The rule names every tag, not just the one: a run that failed holds its reason in Notes as well,
	// and the tag already prints it.
	reason := "the workspace would not start"
	failed := Aggregate{Cases: []Result{{Case: "c", Family: "time", Total: 1, Failed: true, FailReason: reason, Notes: []string{reason}}}}
	if got, want := noteCell(t, Summary(failed), "c"), "FAILED: "+reason; got != want {
		t.Fatalf("a failed run repeated its reason: got %q, want %q", got, want)
	}
}

// TestRescoreKeepsOneOfEachNote pins a rescore's effect on the notes: re-scoring reads the plan and
// the tests again and must not replay the account the run already holds. A note that appears twice
// says nothing new, and one more rescore must not add a third.
func TestRescoreKeepsOneOfEachNote(t *testing.T) {
	old := Result{Case: "c", Family: "time", Total: 1, Notes: []string{"no plan", "the plan copy failed"}}
	scored := func() Result {
		return Result{Case: "c", Family: "time", Total: 1, Notes: []string{"no plan"}}
	}

	once := merge(old, scored())
	if got, want := strings.Join(once.Notes, "; "), "no plan; the plan copy failed"; got != want {
		t.Fatalf("one rescore notes = %q, want %q", got, want)
	}
	twice := merge(once, scored())
	if got, want := strings.Join(twice.Notes, "; "), "no plan; the plan copy failed"; got != want {
		t.Fatalf("two rescores notes = %q, want %q", got, want)
	}
	if got, want := Summary(Aggregate{Cases: []Result{twice}}), Summary(Aggregate{Cases: []Result{old}}); got != want {
		t.Fatalf("a rescore changed the reading:\n%s\nwant:\n%s", got, want)
	}
}
