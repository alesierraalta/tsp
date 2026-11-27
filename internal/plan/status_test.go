package plan

import (
	"strings"
	"testing"
)

// gap-closed records a correct behaviour whose missing test was added: a settled verdict, so it owes
// the pinning test that now holds the behaviour.
func TestGapClosedIsASettledStatusThatOwesAPinningTest(t *testing.T) {
	row := func(test string) string {
		return header + "| F1 | `src/a.js:5` empty input was untested | M | yes | E1 | " + test + " | gap-closed | me | - | - |\n" +
			admittedLedger + admittedRow
	}
	if problems := CheckDocument(row("t.js :: empty input")); len(problems) != 0 {
		t.Fatalf("gap-closed with a pinning test refused: %v", problems)
	}
	problems := CheckDocument(row("-"))
	if len(problems) != 1 || !strings.Contains(problems[0], "names no pinning test") {
		t.Fatalf("gap-closed without a pinning test: want one 'names no pinning test' breach, got %v", problems)
	}
}
