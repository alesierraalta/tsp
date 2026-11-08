package plan

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/alesierraalta/tsp/internal/assets"
)

const reviewCandidates = "\n## Security candidates\n\n| Id | Actor | Authority | Resource | Action | Boundary | Evidence id | Disposition | Observed boundary violation | Missing fact | Safe validation step | Refuting evidence id |\n|---|---|---|---|---|---|---|---|---|---|---|---|\n| S1 | user B | own account | user A record | read | tenant boundary | E1 | confirmed | user A record returned | - | - | - |\n"

const reviewHeader = "\n## Candidate reviews\n\n| Id | Candidate id | Reviewer | Independence attestation | Refutation attempt | Effective controls | Outcome | Decisive evidence id | Blocker |\n|---|---|---|---|---|---|---|---|---|\n"

const reviewRow = "| R1 | S1 | reviewer carol | attested | tried a second tenant and direct id substitution | tenant filter is enforced in the repository layer | confirmed | E1 | - |\n"

// reviewedPlan is the smallest plan the review contract can read: one observed ledger row, one confirmed
// candidate, and one completed independent review of it.
func reviewedPlan() string { return addPlan() + reviewCandidates + reviewHeader + reviewRow }

func TestCandidateReviewsRequireCompleteEvidence(t *testing.T) {
	cases := []struct {
		name string
		row  string
		want string
	}{
		{"a completed review is accepted", reviewRow, ""},
		{"review names its candidate", "| R1 | - | reviewer carol | attested | tried a second tenant | repository tenant filter | confirmed | E1 | - |", "candidate id"},
		{"review must reference a candidate row", "| R1 | S9 | reviewer carol | attested | tried a second tenant | repository tenant filter | confirmed | E1 | - |", "not a row in Security candidates"},
		{"review names its reviewer", "| R1 | S1 | - | attested | tried a second tenant | repository tenant filter | confirmed | E1 | - |", "reviewer"},
		{"independence is attested explicitly", "| R1 | S1 | reviewer carol | - | tried a second tenant | repository tenant filter | confirmed | E1 | - |", "independence attestation"},
		{"attestation is a closed word", "| R1 | S1 | reviewer carol | probably | tried a second tenant | repository tenant filter | confirmed | E1 | - |", "independence attestation"},
		{"review records a refutation attempt", "| R1 | S1 | reviewer carol | attested | - | repository tenant filter | confirmed | E1 | - |", "refutation attempt"},
		{"review evaluates effective controls", "| R1 | S1 | reviewer carol | attested | tried a second tenant | - | confirmed | E1 | - |", "effective controls"},
		{"outcome is a closed word", "| R1 | S1 | reviewer carol | attested | tried a second tenant | repository tenant filter | maybe | E1 | - |", "outcome"},
		{"confirmation cites decisive evidence", "| R1 | S1 | reviewer carol | attested | tried a second tenant | repository tenant filter | confirmed | - | - |", "decisive observed Evidence ledger row"},
		{"confirmation evidence must exist", "| R1 | S1 | reviewer carol | attested | tried a second tenant | repository tenant filter | confirmed | E9 | - |", "E9"},
		{"rejection cites decisive evidence", "| R1 | S1 | reviewer carol | attested | tried a second tenant | repository tenant filter | rejected | - | - |", "decisive observed Evidence ledger row"},
		{"blocked names a specific blocker", "| R1 | S1 | reviewer carol | attested | tried a second tenant | repository tenant filter | blocked | - | - |", "blocker"},
		{"blocked review is complete without decisive evidence", "| R1 | S1 | reviewer carol | attested | tried a second tenant | repository tenant filter | blocked | - | no second tenant fixture |", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			problems := CheckDocument(addPlan() + reviewCandidates + reviewHeader + tc.row + "\n")
			if tc.want == "" && len(problems) != 0 {
				t.Fatalf("unexpected problems: %v", problems)
			}
			if tc.want != "" && !strings.Contains(strings.Join(problems, "\n"), tc.want) {
				t.Fatalf("want a problem containing %q, got %v", tc.want, problems)
			}
		})
	}
}

func TestCandidateReviewsCannotConfirmUnobservedEvidence(t *testing.T) {
	doc := strings.Replace(addPlan(), "| observado |", "| hypothesis |", 1) + reviewCandidates + reviewHeader + reviewRow
	if problems := CheckDocument(doc); !strings.Contains(strings.Join(problems, "\n"), "observado") {
		t.Fatalf("a review confirmed unobserved evidence: %v", problems)
	}
}

// The candidate author's disposition stands on its own. T1 already made a confirmed candidate valid without
// this section, and a review is what makes that confirmation independent — never the other way round.
func TestCandidateReviewIsSeparateFromTheAuthorDisposition(t *testing.T) {
	for _, doc := range []string{
		addPlan() + reviewCandidates,
		reviewedPlan(),
	} {
		if problems := CheckDocument(doc); len(problems) != 0 {
			t.Fatalf("candidate disposition needed a review to be valid: %v", problems)
		}
	}
}

func TestCandidateReviewSectionCannotHideRows(t *testing.T) {
	cases := []struct{ name, body string }{
		{"duplicate section", reviewHeader + reviewRow + reviewHeader + reviewRow},
		{"second table under subheading", reviewHeader + reviewRow + "\n### Notes\n\n" + strings.TrimPrefix(reviewHeader, "\n## Candidate reviews\n\n") + strings.Replace(reviewRow, "| R1 |", "| R2 |", 1)},
		{"placeholder id with populated fields", reviewHeader + strings.Replace(reviewRow, "| R1 |", "| - |", 1)},
		{"duplicate review id", reviewHeader + reviewRow + strings.Replace(reviewRow, "reviewer carol", "reviewer dave", 1)},
		{"interrupted table", reviewHeader + reviewRow + "\nnot a row\n" + strings.Replace(reviewRow, "| R1 |", "| R2 |", 1)},
		{"region ends inside a fence", reviewHeader + reviewRow + "```\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if problems := CheckDocument(addPlan() + reviewCandidates + tc.body); len(problems) == 0 {
				t.Fatal("invalid candidate review was silently ignored")
			}
		})
	}
}

func TestCandidateReviewTableShape(t *testing.T) {
	cases := []struct {
		name string
		doc  string
		want string
	}{
		{"no table", addPlan() + reviewCandidates + "\n## Candidate reviews\n\nR1 was reviewed by carol.\n", "has no table"},
		{"missing reviewer column", addPlan() + reviewCandidates + "\n## Candidate reviews\n\n| Id | Candidate id |\n|---|---|\n| R1 | S1 |\n", "reviewer column"},
		{"repeated outcome column", addPlan() + reviewCandidates + "\n## Candidate reviews\n\n| Id | Candidate id | Reviewer | Independence attestation | Refutation attempt | Effective controls | Outcome | Outcome | Decisive evidence id | Blocker |\n|---|---|---|---|---|---|---|---|---|---|\n| R1 | S1 | reviewer carol | attested | tried a second tenant | repository tenant filter | confirmed | confirmed | E1 | - |\n", "repeats column"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if problems := CheckDocument(tc.doc); !strings.Contains(strings.Join(problems, "\n"), tc.want) {
				t.Fatalf("want a problem containing %q, got %v", tc.want, problems)
			}
		})
	}
}

func TestCandidateReviewSectionIsOptionalAndIgnoresFences(t *testing.T) {
	fenced := addPlan() + reviewCandidates + "\n## Notes\n\n```markdown\n## Candidate reviews\n\n| Id |\n|---|\n```\n"
	for _, doc := range []string{addPlan(), addPlan() + reviewCandidates, fenced} {
		if problems := CheckDocument(doc); len(problems) != 0 {
			t.Fatalf("fenced or absent review section was read as a contract: %v", problems)
		}
	}
}

// The shipped template owns the documented shape, so it has to carry the section with exactly the columns
// the checker reads. Header-only is deliberate: `plan init` writes no review and no confirmation.
func TestCandidateReviewTemplateShipsTheDocumentedShape(t *testing.T) {
	raw, err := fs.ReadFile(assets.Skills(), TemplatePath)
	if err != nil {
		t.Fatal(err)
	}
	header, rows := Table(string(raw), securityReviewsName)
	if header == nil {
		t.Fatalf("the template has no %s table", securityReviewsName)
	}
	if len(rows) != 0 {
		t.Fatalf("the template must not ship a review, got %v", rows)
	}
	for _, want := range securityReviewColumns {
		found := false
		for _, h := range header {
			if strings.EqualFold(strings.TrimSpace(h), want) {
				found = true
			}
		}
		if !found {
			t.Fatalf("the template review table has no %q column: %v", want, header)
		}
	}
}
