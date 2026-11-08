package plan

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const criteriaHeader = "## Request criteria\n\n| Id | Criterion (one line) | Disposition | Source | Reason |\n|---|---|---|---|---|\n"

func TestRequestCriteria(t *testing.T) {
	template, err := Template()
	if err != nil {
		t.Fatal(err)
	}
	request := "## Request\n\n```\nDeliver it\n```\n\n"
	for _, tc := range []struct{ name, prefix, rows, want string }{
		{"dangling", request, "| C1 | Deliver it | covered | E99 | - |\n", "C1"},
		{"reason", request, "| C2 | Deliver it | blocked | - | - |\n", "C2"},
		{"status", request, "| C3 | Deliver it | maybe | - | - |\n", "C3"},
		{"duplicate", request, "| C4 | Deliver it | na | - | irrelevant |\n| C4 | Again | na | - | irrelevant |\n", "C4"},
		{"no request", "", "| C5 | Deliver it | na | - | irrelevant |\n", "Request"},
		{"valid", request, "| C6 | Deliver it | na | - | irrelevant |\n", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			problems := strings.Join(CheckDocument(tc.prefix+criteriaHeader+tc.rows+"\n"+template), "\n")
			if tc.want == "" && problems != "" || tc.want != "" && !strings.Contains(problems, tc.want) {
				t.Fatalf("problems: %s", problems)
			}
		})
	}
}
func TestRequestInitVerbatim(t *testing.T) {
	p := filepath.Join(t.TempDir(), "plan.md")
	request := []byte("## Findings\n```\n| raw | bytes |\n```")
	if err := InitRequest(p, "", false, false, request); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), string(request)) || !strings.Contains(string(b), criteriaHeader[:19]) {
		t.Fatalf("missing request or table: %s", b)
	}
	if problems := CheckDocument(string(b)); len(problems) != 0 {
		t.Fatalf("request changed structure: %v", problems)
	}
}
func TestInitWithoutRequestUnchanged(t *testing.T) {
	p := filepath.Join(t.TempDir(), "plan.md")
	if err := Init(p, "", false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	want, err := Template()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want || fmt.Sprintf("%x", sha256.Sum256(got)) != "7a7715a74ce82f192e4e0b0302663da2e3bd6686a63f3c3505ccdafd75b203b8" {
		t.Fatal("init without --spec changed template bytes")
	}
}

func TestRequestGaps(t *testing.T) {
	doc := "## Request\n\ntext\n\n" + criteriaHeader + "| C1 | x | blocked | - | waiting |\n| C2 | x | | - | - |\n| C3 | x | covered | E1 | - |\n| C4 | x | na | - | no |\n\n## Evidence ledger\n| Id | Label |\n|---|---|\n| E1 | observado |\n"
	g, err := GapsIn(doc)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.RequestOwed) != 2 {
		t.Fatalf("owed: %v", g.RequestOwed)
	}
}
