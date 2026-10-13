package bench

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFalsePositiveRequiresAdjudication(t *testing.T) {
	key := Key{ID: "case-95", Defects: []Defect{{ID: "D1", File: "src/render.js", Line: 12, Keywords: []string{"escape"}}}}
	finding := "| F1 | src/other.js:3 unrelated finding | M | yes | | open | me | - | - |\n"
	p := plan(finding, "")

	r := Score(p, key)
	if r.FalsePositives != 0 || r.PendingAdjudication != 1 || r.Precision != nil {
		t.Fatalf("mechanical score = %+v, want no false positive, one pending row, and undefined precision", r)
	}

	adj := adjudicationFor(t, p, key, 1, VerdictFalsePositive, "")
	r, err := ScoreAdjudicated(p, key, &adj)
	if err != nil {
		t.Fatal(err)
	}
	if r.FalsePositives != 1 || r.AdjudicatedFalse != 1 || r.PendingAdjudication != 0 || !r.AdjudicationComplete {
		t.Fatalf("adjudicated score = %+v, want one adjudicated false positive and no pending rows", r)
	}
	if r.Precision == nil || *r.Precision != 0 {
		t.Fatalf("precision = %v, want 0", r.Precision)
	}
}

func TestProposedMatchesRemainPendingWithoutARecord(t *testing.T) {
	key := Key{ID: "case-95", Defects: []Defect{{ID: "D1", File: "src/render.js", Line: 12, Keywords: []string{"escape"}}}}
	p := plan("| F1 | src/render.js:12 escape | M | yes | E1 | open | me | - | - |\n| F2 | src/other.js:3 unrelated finding | M | yes | E2 | open | me | - | - |\n", "| E1 | c | cmd | i | o | m | r | observed |\n| E2 | c | cmd | i | o | m | r | observed |\n")

	r := Score(p, key)
	if r.Found != 1 || r.UnmatchedFindings != 1 || r.PendingAdjudication != 2 || r.Precision != nil {
		t.Fatalf("score = %+v, want one proposed match, one unmatched row, and two pending rows", r)
	}
}

func TestAdjudicatedDefectCanConfirmAnUnproposedDefect(t *testing.T) {
	key := Key{ID: "case-95", Defects: []Defect{{ID: "D1", File: "src/render.js", Line: 12, Keywords: []string{"escape"}}}}
	p := plan("| F1 | src/other.js:3 unrelated finding | M | yes | | open | me | - | - |\n", "")
	adj := adjudicationFor(t, p, key, 1, VerdictDefect, "D1")

	r, err := ScoreAdjudicated(p, key, &adj)
	if err != nil {
		t.Fatal(err)
	}
	if r.Found != 0 || r.AdjudicatedTrue != 1 || r.Defects[0].Found || !r.Defects[0].Confirmed {
		t.Fatalf("score = %+v, want a confirmed defect without changing mechanical found", r)
	}
}

func TestOutOfScopeDoesNotEnterPrecision(t *testing.T) {
	key := Key{ID: "case-95", Defects: []Defect{{ID: "D1", File: "src/render.js", Line: 12, Keywords: []string{"escape"}}}}
	p := plan("| F1 | docs/style.md:3 wording suggestion | N | yes | | open | me | - | - |\n", "")
	adj := adjudicationFor(t, p, key, 1, VerdictOutOfScope, "")

	r, err := ScoreAdjudicated(p, key, &adj)
	if err != nil {
		t.Fatal(err)
	}
	if r.OutOfScope != 1 || r.FalsePositives != 0 || r.Precision != nil || !r.AdjudicationComplete {
		t.Fatalf("score = %+v, want one out-of-scope row, no false positives, and undefined precision", r)
	}
}

func TestStaleAdjudicationFingerprintIsRefused(t *testing.T) {
	key := Key{ID: "case-95", Defects: []Defect{{ID: "D1", File: "src/render.js", Line: 12, Keywords: []string{"escape"}}}}
	p := plan("| F1 | src/other.js:3 unrelated finding | M | yes | | open | me | - | - |\n", "")
	adj := Adjudication{Case: key.ID, Run: 1, Decisions: []Decision{{
		Row: 1, RowFingerprint: "stale", Verdict: VerdictFalsePositive, By: "reviewer", TS: "2025-01-01T00:00:00Z", Reason: "confirmed",
	}}}

	_, err := ScoreAdjudicated(p, key, &adj)
	if err == nil || !strings.Contains(err.Error(), "row 1") || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("error = %v, want a stale decision error naming row 1", err)
	}
}

func TestAdjudicationRejectsDuplicateUnknownAndMismatchedDecisions(t *testing.T) {
	key := Key{ID: "case-95", Defects: []Defect{{ID: "D1", File: "src/render.js", Line: 12, Keywords: []string{"escape"}}}, Traps: []Trap{{ID: "t1", File: "src/render.js", Line: 20, Keywords: []string{"jitter"}, Documented: "README.md:9"}}}
	p := plan("| F1 | src/other.js:3 unrelated finding | M | yes | | open | me | - | - |\n", "")
	fp := findingFingerprint(t, p, 1)

	tests := []struct {
		name string
		adj  Adjudication
		want string
	}{
		{
			name: "duplicate row",
			adj: Adjudication{Case: key.ID, Run: 1, Decisions: []Decision{
				decision(1, fp, VerdictFalsePositive, "", "first"),
				decision(1, fp, VerdictOutOfScope, "", "second"),
			}},
			want: "row 1",
		},
		{
			name: "unknown verdict",
			adj:  Adjudication{Case: key.ID, Run: 1, Decisions: []Decision{decision(1, fp, "unknown", "", "reason")}},
			want: "unknown",
		},
		{
			name: "unknown defect id",
			adj:  Adjudication{Case: key.ID, Run: 1, Decisions: []Decision{decision(1, fp, VerdictDefect, "D9", "reason")}},
			want: "D9",
		},
		{
			name: "trap id is never confirmable as a defect",
			adj:  Adjudication{Case: key.ID, Run: 1, Decisions: []Decision{decision(1, fp, VerdictDefect, "t1", "reason")}},
			want: "trap",
		},
		{
			name: "wrong case",
			adj:  Adjudication{Case: "other-case", Run: 1, Decisions: []Decision{decision(1, fp, VerdictFalsePositive, "", "reason")}},
			want: "other-case",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ScoreAdjudicated(p, key, &tt.adj)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestAdjudicationValidationReportsEveryProblem(t *testing.T) {
	adj := Adjudication{
		Case: " ", Run: 0, Decisions: []Decision{
			{Row: 0, RowFingerprint: "", Verdict: "mystery", Defect: "D1", By: "", Reason: ""},
			{Row: 1, RowFingerprint: "fp", Verdict: VerdictFalsePositive, Defect: "D1", By: "reviewer", Reason: "reason"},
			{Row: 1, RowFingerprint: "fp", Verdict: VerdictFalsePositive, By: "reviewer", Reason: "reason"},
		},
	}

	err := adj.Validate()
	if err == nil {
		t.Fatal("Validate() succeeded for an invalid adjudication")
	}
	for _, want := range []string{"case", "run", "row 0", "mystery", "defect", "by", "reason", "row_fingerprint", "row 1"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("validation error = %q, want it to contain %q", err, want)
		}
	}
}

func TestUndefinedPrecisionRoundTripsAsJSONNull(t *testing.T) {
	key := Key{ID: "case-95", Defects: []Defect{{ID: "D1", File: "src/render.js", Line: 12, Keywords: []string{"escape"}}}}
	r := Score(plan("| F1 | docs/style.md:3 wording suggestion | N | yes | | open | me | - | - |\n", ""), key)
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte(`"precision":null`)) {
		t.Fatalf("json = %s, want precision null", raw)
	}
	var decoded Result
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Precision != nil {
		t.Fatalf("decoded precision = %v, want nil", decoded.Precision)
	}
}

func adjudicationFor(t *testing.T, planText string, key Key, row int, verdict, defect string) Adjudication {
	t.Helper()
	return Adjudication{Case: key.ID, Run: 1, Decisions: []Decision{decision(row, findingFingerprint(t, planText, row), verdict, defect, "reviewed")}}
}

func findingFingerprint(t *testing.T, planText string, row int) string {
	t.Helper()
	fingerprint, err := FindingRowFingerprint(planText, row)
	if err != nil {
		t.Fatal(err)
	}
	return fingerprint
}

func decision(row int, fingerprint, verdict, defect, reason string) Decision {
	return Decision{Row: row, RowFingerprint: fingerprint, Verdict: verdict, Defect: defect, By: "reviewer", TS: "2025-01-01T00:00:00Z", Reason: reason}
}

// A record is read from the path the caller names, never discovered: the plan a workspace holds sits
// in the area the evaluated subject writes, so a record found beside it would let the subject rule on
// its own findings. A missing file is an error here because the caller asked for it by name.
func TestLoadAdjudicationRefusesAMissingOrMalformedRecord(t *testing.T) {
	dir := t.TempDir()
	if _, err := LoadAdjudication(filepath.Join(dir, AdjudicationFile)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing record error = %v, want it to say the file is not there", err)
	}
	sidecar := filepath.Join(dir, AdjudicationFile)
	if err := os.WriteFile(sidecar, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadAdjudication(sidecar); err == nil {
		t.Fatal("a malformed record was accepted")
	}
	record := Adjudication{Case: "c1", Run: 1, Decisions: []Decision{decision(1, "fp", VerdictFalsePositive, "", "reason")}}
	if err := writeJSON(sidecar, record); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadAdjudication(sidecar)
	if err != nil || loaded.Case != "c1" || len(loaded.Decisions) != 1 {
		t.Fatalf("loaded = %+v, %v", loaded, err)
	}
}

// A decision that displaces another keeps it: the record is the audit trail of what was judged, not
// only of what stands now.
func TestDecideReplacesAndRefusesDuplicateRows(t *testing.T) {
	record := Adjudication{Case: "c1", Run: 1}
	first := decision(1, "fp", VerdictOutOfScope, "", "style, not a defect")
	if err := record.Decide(first, false); err != nil {
		t.Fatal(err)
	}
	if err := record.Decide(decision(1, "fp", VerdictFalsePositive, "", "closer look"), false); err == nil || !strings.Contains(err.Error(), "row 1") {
		t.Fatalf("duplicate row error = %v, want it to name row 1", err)
	}
	if err := record.Decide(decision(1, "fp", VerdictFalsePositive, "", "closer look"), true); err != nil {
		t.Fatal(err)
	}
	if len(record.Decisions) != 1 || record.Decisions[0].Verdict != VerdictFalsePositive {
		t.Fatalf("decisions = %+v, want the replacement to stand alone", record.Decisions)
	}
	if len(record.Superseded) != 1 || record.Superseded[0].Verdict != VerdictOutOfScope {
		t.Fatalf("superseded = %+v, want the displaced decision kept", record.Superseded)
	}
	if err := record.Validate(); err != nil {
		t.Fatalf("record with a superseded entry no longer validates: %v", err)
	}
}

// The writer holds the same rules the reader does, so a record cannot be written that the scorer
// would later refuse to read.
func TestDecideValidatesTheRecordItWrites(t *testing.T) {
	record := Adjudication{Case: "c1", Run: 1}
	if err := record.Decide(Decision{Row: 1, RowFingerprint: "fp", Verdict: VerdictFalsePositive, Reason: "no actor"}, false); err == nil {
		t.Fatal("a decision with no actor was written")
	}
	if len(record.Decisions) != 0 {
		t.Fatalf("refused decision was recorded anyway: %+v", record.Decisions)
	}
}
