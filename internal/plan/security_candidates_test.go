package plan

import (
	"strings"
	"testing"
)

func TestSecurityCandidateDispositions(t *testing.T) {
	header := "\n## Security candidates\n\n| Id | Actor | Authority | Resource | Action | Boundary | Evidence id | Disposition | Observed boundary violation | Missing fact | Safe validation step | Refuting evidence id |\n|---|---|---|---|---|---|---|---|---|---|---|---|\n"
	cases := []struct {
		name string
		row  string
		want string
	}{
		{"confirmed has observed boundary violation", "| S1 | user B | own account | user A record | read | tenant boundary | E1 | confirmed | user A record returned | - | - | - |", ""},
		{"confirmed without effect is not confirmed", "| S1 | user B | own account | user A record | read | tenant boundary | E1 | confirmed | - | - | - | - |", "observed boundary violation"},
		{"confirmed requires recorded evidence", "| S1 | user B | own account | user A record | read | tenant boundary | E9 | confirmed | record returned | - | - | - |", "E9"},
		{"blocked explains safe next step", "| S1 | user B | own account | user A record | read | tenant boundary | - | blocked | - | test identity | create synthetic identities | - |", ""},
		{"blocked requires missing fact", "| S1 | user B | own account | user A record | read | tenant boundary | - | blocked | - | - | create synthetic identities | - |", "missing fact"},
		{"blocked requires safe validation", "| S1 | user B | own account | user A record | read | tenant boundary | - | blocked | - | test identity | - | - |", "safe validation step"},
		{"rejected cites refuting evidence", "| S1 | user B | own account | user A record | read | tenant boundary | - | rejected | - | - | - | E1 |", ""},
		{"rejected cannot cite missing evidence", "| S1 | user B | own account | user A record | read | tenant boundary | - | rejected | - | - | - | E9 |", "E9"},
		{"rejected requires refuting evidence", "| S1 | user B | own account | user A record | read | tenant boundary | - | rejected | - | - | - | - |", "refuting evidence id"},
		{"candidate names the boundary", "| S1 | user B | own account | user A record | read | - | E1 | confirmed | record returned | - | - | - |", "boundary"},
		{"unknown disposition is refused", "| S1 | user B | own account | user A record | read | tenant boundary | E1 | maybe | record returned | - | - | - |", "disposition"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			problems := CheckDocument(addPlan() + header + tc.row + "\n")
			if tc.want == "" && len(problems) != 0 {
				t.Fatalf("unexpected problems: %v", problems)
			}
			if tc.want != "" && !strings.Contains(strings.Join(problems, "\n"), tc.want) {
				t.Fatalf("want a problem containing %q, got %v", tc.want, problems)
			}
		})
	}
}

func TestSecurityCandidateEvidenceMustBeObserved(t *testing.T) {
	doc := strings.Replace(addPlan(), "| observado |", "| hypothesis |", 1) + "\n## Security candidates\n\n| Id | Actor | Authority | Resource | Action | Boundary | Evidence id | Disposition | Observed boundary violation | Missing fact | Safe validation step | Refuting evidence id |\n|---|---|---|---|---|---|---|---|---|---|---|---|\n| S1 | user B | own account | user A record | read | tenant boundary | E1 | confirmed | record returned | - | - | - |\n"
	if problems := CheckDocument(doc); !strings.Contains(strings.Join(problems, "\n"), "observado") {
		t.Fatalf("unobserved evidence was accepted: %v", problems)
	}
}

func TestSecurityCandidateSectionCannotHideRows(t *testing.T) {
	const table = "\n## Security candidates\n\n| Id | Actor | Authority | Resource | Action | Boundary | Evidence id | Disposition | Observed boundary violation | Missing fact | Safe validation step | Refuting evidence id |\n|---|---|---|---|---|---|---|---|---|---|---|---|\n"
	const bad = "| S9 | user B | own account | record | read | boundary | E9 | confirmed | - | - | - | - |\n"
	cases := []struct{ name, body string }{
		{"duplicate section", table + table + bad},
		{"second table under subheading", table + "\n### Notes\n\n" + strings.TrimPrefix(table, "\n## Security candidates\n\n") + bad},
		{"placeholder id with populated fields", table + strings.Replace(bad, "| S9 |", "| - |", 1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if problems := CheckDocument(addPlan() + tc.body); len(problems) == 0 {
				t.Fatal("invalid security candidate was silently ignored")
			}
		})
	}
}

func TestSecurityCandidateExactEvidenceColumns(t *testing.T) {
	const table = "\n## Security candidates\n\n| Id | Actor | Authority | Resource | Action | Boundary | Evidence id | Disposition | Observed boundary violation | Missing fact | Safe validation step | Refuting evidence id |\n|---|---|---|---|---|---|---|---|---|---|---|---|\n| S1 | user B | own account | record | read | boundary | E1 | confirmed | record returned | - | - | - |\n"
	cases := []struct{ name, doc string }{
		{"label in a trailing column", strings.ReplaceAll(strings.Replace(addPlan(), "| Label |", "| Label | Extra |", 1), "| observado |", "| razonado | observado |")},
		{"not executed is not executed", strings.Replace(addPlan(), "| Executed |", "| Not executed |", 1)},
		{"negated executed suffix", strings.Replace(addPlan(), "| Executed |", "| Executed (not executed) |", 1)},
		{"negated observed suffix", strings.Replace(addPlan(), "| Observed |", "| Observed (not observed) |", 1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if problems := CheckDocument(tc.doc + table); len(problems) == 0 {
				t.Fatal("non-observed ledger row was accepted")
			}
		})
	}
}

func TestSecurityCandidateSupportsTemplateLedgerHeaders(t *testing.T) {
	const table = "\n## Security candidates\n\n| Id | Actor | Authority | Resource | Action | Boundary | Evidence id | Disposition | Observed boundary violation | Missing fact | Safe validation step | Refuting evidence id |\n|---|---|---|---|---|---|---|---|---|---|---|---|\n| S1 | user B | own account | record | read | boundary | E1 | confirmed | record returned | - | - | - |\n"
	doc := strings.Replace(addPlan(), "| Label |", "| Label (`observado` / `razonado`, literal) |", 1)
	doc = strings.Replace(doc, "| Executed |", "| Executed (prose) |", 1)
	if problems := CheckDocument(doc + table); len(problems) != 0 {
		t.Fatalf("valid template ledger rejected: %v", problems)
	}
}

func TestSecurityCandidatesAreOptionalForExistingPlans(t *testing.T) {
	for _, doc := range []string{addPlan(), addPlan() + "\n## Notes\n\n```markdown\n## Security candidates\n\n| Id | Actor |\n|---|---|\n```\n"} {
		if problems := CheckDocument(doc); len(problems) != 0 {
			t.Fatalf("existing plan rejected: %v", problems)
		}
	}
}
