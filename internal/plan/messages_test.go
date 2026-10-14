package plan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A refusal is followed by a retry, so it says exactly what to write. Hard-suite runs wrote `pending` as the
// Baseline fingerprint and a `Light:` line with no touched classes, and each spent turns guessing the accepted
// form the refusal never named.
func TestCheckRefusalsNameTheAcceptedForm(t *testing.T) {
	cases := []struct {
		name, from, to string
		want           []string
	}{
		{
			name: "a Baseline fingerprint fingerprint.sh did not write",
			from: "`<assets/fingerprint.sh output>`", to: "`pending`",
			want: []string{`the Baseline fingerprint "pending"`, "`<assets/fingerprint.sh output>`", "test-strategy/assets/fingerprint.sh"},
		},
		{
			name: "a Light declaration with no touched classes",
			from: "Tables are the format", to: "Light: src/a.js · touches -\n\nTables are the format",
			want: []string{"the Light declaration names no touched classes", "`Light: <blast radius> · touches <classes>`"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "plan.md")
			if err := Init(p, "", false); err != nil {
				t.Fatal(err)
			}
			body, _ := os.ReadFile(p)
			if !strings.Contains(string(body), tc.from) {
				t.Fatalf("the template no longer holds %q", tc.from)
			}
			problems := CheckDocument(strings.Replace(string(body), tc.from, tc.to, 1))
			got := strings.Join(problems, "\n")
			for _, w := range tc.want {
				if !strings.Contains(got, w) {
					t.Errorf("refusal missing %q:\n%s", w, got)
				}
			}
		})
	}
}
