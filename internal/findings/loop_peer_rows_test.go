package findings_test

import (
	"strings"
	"testing"

	"github.com/alesierraalta/tsp/internal/findings"
)

// A rejected witness is not reopened by a different test that carries its name: the user's verdict stays.
func TestASameNamedTestElsewhereKeepsTheVerdict(t *testing.T) {
	d := repo(t)
	put(t, d, "value_test.go", redTest)
	if out, code := recordSuite(t, d, "go test -count=1 ./..."); code != 0 {
		t.Fatalf("record: %d %q", code, out)
	}
	if out, code := findings.SetVerdict(d, "F1", "rejected", "the promise was invented"); code != 0 {
		t.Fatalf("verdict: %d %q", code, out)
	}
	command(t, d, "git", "rm", "-q", "value_test.go")
	put(t, d, "sub/other_test.go", "package sub\n\nimport \"testing\"\n\nfunc TestPromise(t *testing.T) { t.Fatal(\"another test, same name\") }\n")
	a := next(t, d, 1)
	for _, it := range a.Items {
		if it.ID == "F1" {
			t.Fatalf("the user's verdict was reopened: %+v", a.Items)
		}
	}
	if got := storedFindings(t)[0]; got.Status != "rejected" || got.Verdict == nil {
		t.Fatalf("stored finding = %+v, want the rejection kept", got)
	}
	found := false
	for _, it := range a.Items {
		if it.Status == "failing" && strings.Contains(it.Test, "TestPromise") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the different failing test is still a failure: %+v", a.Items)
	}
}
