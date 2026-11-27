package plan

import (
	"strings"
	"testing"
)

// The template already says the ledger holds one row per conclusion executed and observed, and that a
// `razonado` item goes under Hypotheses. `plan check` enforced none of it: a Findings row could claim a
// verdict over an evidence row whose Admit and Digest cells were never filled, so hand-written evidence
// passed the checker that exists to refuse it. These tests are the rule the template states.

const admittedFindings = "## Findings\n\n" +
	"| Id | Finding | Severity | Data safe? | Evidence id | Pinning test | Status | Verdict by / date | Reason |\n" +
	"|---|---|---|---|---|---|---|---|---|\n"

const admittedLedgerHeader = "\n## Evidence ledger\n\n" +
	"| Id | Claim | Executed | Admit | Inputs | Observed | Digest | Normalize | Mode | Mutation | Reproduction | Label |\n" +
	"|---|---|---|---|---|---|---|---|---|---|---|---|\n"

// admittedLedgerRow is one ledger row with the three admission cells the rule reads, so a case names only
// the cells it is about.
func admittedLedgerRow(admit, digest, mode string) string {
	return "| E1 | it drops the comma | `node --test` | " + admit + " | `a,\"b,c\"` | 3 fields | " + digest +
		" | | " + mode + " | reverted → red | same input | observado |\n"
}

// admittedPlan is one Findings row citing one ledger row, with both rows' cells supplied by the case.
func admittedPlan(status, admit, digest, mode string) string {
	return admittedFindings +
		"| F1 | `src/a.js:5` drops a quoted comma | data loss | yes | E1 | t.js :: keeps a comma | " + status + " | me / 2026-09-10 | - |\n" +
		admittedLedgerHeader + admittedLedgerRow(admit, digest, mode)
}

// A verdict claimed in Findings is worth exactly what the evidence row it cites recorded, so a settled
// finding over an unadmitted row is refused rather than counted as verified.
func TestCheckRefusesASettledFindingBackedByUnadmittedEvidence(t *testing.T) {
	for _, status := range []string{"confirmed", "fixed", "gap-closed"} {
		t.Run(status, func(t *testing.T) {
			problems := CheckDocument(admittedPlan(status, "", "", ""))
			joined := strings.Join(problems, "\n")
			for _, want := range []string{"finding F1", "E1", "Admit", "Digest", "Mode", "tsp plan admit --execute --record E1"} {
				if !strings.Contains(joined, want) {
					t.Fatalf("%s finding over unadmitted evidence: problems = %v, want one naming %q", status, problems, want)
				}
			}
		})
	}
}

// The same plan, once the row it cites carries what `plan admit --execute --record` writes, passes: the rule
// refuses an empty record, never evidence that was observed.
func TestCheckAcceptsASettledFindingOverAdmittedEvidence(t *testing.T) {
	for _, status := range []string{"confirmed", "fixed", "gap-closed"} {
		t.Run(status, func(t *testing.T) {
			if problems := CheckDocument(admittedPlan(status, "node --test t.js", fixtureDigest, "host")); len(problems) != 0 {
				t.Fatalf("an admitted evidence row was refused: %v", problems)
			}
			if problems := CheckDocument(admittedPlan(status, "node --test t.js", fixtureDigest, "sandbox")); len(problems) != 0 {
				t.Fatalf("an evidence row admitted in sandbox mode was refused: %v", problems)
			}
		})
	}
}

// `open` is the one status that claims nothing, so an open finding may cite a row that was never admitted.
func TestCheckLeavesAnOpenFindingUnverified(t *testing.T) {
	if problems := CheckDocument(admittedPlan("open", "", "", "")); len(problems) != 0 {
		t.Fatalf("an open finding owes no admitted evidence, and was refused: %v", problems)
	}
}

// A plan that concluded nothing claims nothing: the shipped skeleton passes exactly as before.
func TestCheckAcceptsAPlanWithNoFindings(t *testing.T) {
	template, err := Template()
	if err != nil {
		t.Fatal(err)
	}
	if problems := CheckDocument(template); len(problems) != 0 {
		t.Fatalf("the shipped skeleton is refused: %v", problems)
	}
	empty := "## Findings\n\n| Id | Finding | Status | Evidence id |\n|---|---|---|---|\n\n## Evidence ledger\n\n| Id | Claim | Admit | Digest | Mode |\n|---|---|---|---|---|\n"
	if problems := CheckDocument(empty); len(problems) != 0 {
		t.Fatalf("a plan with no findings is refused: %v", problems)
	}
}

// Half an admission is not an admission: a digest with no command behind it records an output nothing says
// how to reproduce, and a command with no digest never observed one.
func TestCheckRefusesHalfAdmittedEvidence(t *testing.T) {
	cases := []struct{ name, admit, digest, mode string }{
		{"a digest with no command", "", fixtureDigest, "host"},
		{"a command with no digest", "node --test t.js", "", "host"},
		{"an admission with no mode", "node --test t.js", fixtureDigest, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			problems := CheckDocument(admittedPlan("fixed", tc.admit, tc.digest, tc.mode))
			joined := strings.Join(problems, "\n")
			if !strings.Contains(joined, "finding F1") || !strings.Contains(joined, "tsp plan admit --execute --record E1") {
				t.Fatalf("%s: problems = %v, want a refusal naming finding F1 and the command that fixes it", tc.name, problems)
			}
		})
	}
}

// A Digest cell is only a pin when it is the digest `plan admit` writes, and a Mode cell only means something
// when it says where: a hand-written cell in either is refused by the shape that records one.
func TestCheckRefusesADigestOrModeItCannotRead(t *testing.T) {
	cases := []struct{ name, admit, digest, mode string }{
		{"a hand-written digest", "node --test t.js", "the tests passed", "host"},
		{"a truncated digest", "node --test t.js", "sha256:7eada7", "host"},
		{"a mode that names no place", "node --test t.js", fixtureDigest, "container"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			problems := CheckDocument(admittedPlan("fixed", tc.admit, tc.digest, tc.mode))
			if len(problems) == 0 {
				t.Fatalf("%s: the evidence cell was read as an admission", tc.name)
			}
		})
	}
}

// The refusal is a message an operator can act on: it names the file the CLI prints, the finding, the cell,
// and the one command that fills it.
func TestCheckNamesTheFindingAndTheCommandThatAdmitsIt(t *testing.T) {
	dir := t.TempDir()
	p := write(t, dir, "plan.md", admittedPlan("fixed", "", "", ""))
	problems, err := Check(p)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(problems, "\n")
	if !strings.Contains(joined, "line 5: finding F1") {
		t.Fatalf("the refusal must name the row's line and finding: %v", problems)
	}
	if !strings.Contains(joined, "tsp plan admit --execute --record E1") {
		t.Fatalf("the refusal must name the command that fixes it: %v", problems)
	}
}
