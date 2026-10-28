package bench

import (
	"strings"
	"testing"
)

// TestSelectCasesAimsByFamily pins the selection: a named family keeps only the cases that declare it, so a run
// spends its budget on the failure kind it is aimed at, and no selection keeps every case.
func TestSelectCasesAimsByFamily(t *testing.T) {
	all := vendoredCases(t)
	if len(all) == 0 {
		t.Fatal("no vendored cases")
	}
	kept, err := selectCases(all, []string{"security"})
	if err != nil {
		t.Fatalf("selectCases security: %v", err)
	}
	if len(kept) == 0 || len(kept) >= len(all) {
		t.Fatalf("security kept %d of %d cases, want a nonempty proper subset", len(kept), len(all))
	}
	for _, dir := range kept {
		key, err := LoadKey(dir)
		if err != nil {
			t.Fatalf("kept %s: %v", dir, err)
		}
		if key.Family != "security" {
			t.Fatalf("kept %s declaring family %q, want security", dir, key.Family)
		}
	}
	every, err := selectCases(all, nil)
	if err != nil || len(every) != len(all) {
		t.Fatalf("selectCases(nil) = %d cases, %v; want all %d", len(every), err, len(all))
	}
}

// TestSelectCasesRefusesANameOutsideFamilies keeps a typo from publishing a green run over nothing: an unknown
// family and a selection no case declares are both refused, and the refusal names what the key accepts.
func TestSelectCasesRefusesANameOutsideFamilies(t *testing.T) {
	all := vendoredCases(t)
	if _, err := selectCases(all, []string{"securty"}); err == nil {
		t.Fatal("selectCases accepted a misspelled family")
	} else if !strings.Contains(err.Error(), "not one of") || !strings.Contains(err.Error(), "security") {
		t.Fatalf("refusal does not name the accepted families: %v", err)
	}
	if _, err := selectCases(nil, []string{"security"}); err == nil {
		t.Fatal("selectCases accepted a selection no case declares")
	}
	if _, err := selectCases(all, []string{" ", ""}); err == nil {
		t.Fatal("selectCases accepted a blank selection")
	}
}
