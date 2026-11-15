package report_test

import (
	"strings"
	"testing"
	"time"

	"example.com/docvault/internal/report"
	"example.com/docvault/internal/vault"
)

var now = time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)

func docs() []vault.Doc {
	return []vault.Doc{
		{
			ID:      "doc-000001",
			Key:     "documents/one.txt",
			Tenant:  "acme",
			Title:   "Quarterly <report>",
			Kind:    "article",
			Owner:   "amara",
			Tags:    []string{"finance", "<b>urgent</b>"},
			Size:    12,
			Updated: now,
		},
		{
			ID:      "doc-000002",
			Key:     "documents/two.txt",
			Tenant:  "acme",
			Title:   "Standup notes",
			Kind:    "note",
			Owner:   "amara",
			Size:    4,
			Updated: now,
		},
	}
}

func TestTallyCountsKindsAndBytes(t *testing.T) {
	c := report.Tally(docs())
	if c.Total != 2 || c.ByKind["article"] != 1 || c.ByKind["note"] != 1 || c.Bytes != 16 {
		t.Fatalf("tally = %+v", c)
	}
	if got := report.KindBreakdown(c); got != "article=1\nnote=1\n" {
		t.Fatalf("breakdown = %q", got)
	}
}

func TestIndexEscapesEveryDocumentValue(t *testing.T) {
	body := report.IndexHTML(docs(), now)
	if strings.Contains(body, "<report>") || strings.Contains(body, "<b>urgent</b>") {
		t.Fatalf("the index wrote raw document text: %s", body)
	}
	if !strings.Contains(body, "&lt;report&gt;") {
		t.Fatalf("the index lost the escaped title: %s", body)
	}
	if !strings.Contains(body, "2 documents, 16 bytes") {
		t.Fatalf("the index lost its counts: %s", body)
	}
}

func TestSummaryEscapesArticleText(t *testing.T) {
	article := docs()[0]
	body := report.SummaryHTML(article, []byte("<script>alert(1)</script>"), now)
	if strings.Contains(body, "<script>") || strings.Contains(body, "<report>") {
		t.Fatalf("the summary wrote raw document text: %s", body)
	}
	if !strings.Contains(body, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Fatalf("the summary lost the escaped body: %s", body)
	}
	if !strings.Contains(body, "<h1>Quarterly &lt;report&gt;</h1>") {
		t.Fatalf("the summary lost the escaped heading: %s", body)
	}
}

func TestSummaryCarriesTheMetadata(t *testing.T) {
	body := report.SummaryHTML(docs()[1], nil, now)
	for _, want := range []string{"doc-000002", "acme", "note", "amara"} {
		if !strings.Contains(body, want) {
			t.Fatalf("the summary does not carry %q: %s", want, body)
		}
	}
}

func TestCSVDefusesFormulasInEveryColumn(t *testing.T) {
	doc := vault.Doc{
		ID:      "=cmd|' /C calc'!A0",
		Key:     "documents/one.txt",
		Tenant:  "acme",
		Title:   "+1+1",
		Kind:    "article",
		Owner:   "@sum",
		Tags:    []string{"-2+2"},
		Updated: now,
	}
	body, err := report.CSV([]vault.Doc{doc})
	if err != nil {
		t.Fatalf("csv: %v", err)
	}
	for _, live := range []string{",=cmd", ",+1+1", ",@sum", ",-2+2"} {
		if strings.Contains(body, live) {
			t.Fatalf("the export left %q live: %s", live, body)
		}
	}
	for _, defused := range []string{"'=cmd", "'+1+1", "'@sum", "'-2+2"} {
		if !strings.Contains(body, defused) {
			t.Fatalf("the export does not carry %q: %s", defused, body)
		}
	}
}

func TestCSVCarriesTheHeaderAndOneRowPerDocument(t *testing.T) {
	body, err := report.CSV(docs())
	if err != nil {
		t.Fatalf("csv: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(body), "\n")
	if len(lines) != 3 {
		t.Fatalf("csv has %d lines, want a header and two rows: %s", len(lines), body)
	}
	if !strings.HasPrefix(lines[0], "id,key,tenant") {
		t.Fatalf("header = %q", lines[0])
	}
}

func TestRowsAppliesTheSameCellRule(t *testing.T) {
	rows := report.Rows([]vault.Doc{{ID: "=evil", Tenant: "acme", Kind: "note", Title: "t"}})
	if len(rows) != 1 || rows[0][0] != "'=evil" {
		t.Fatalf("rows = %v", rows)
	}
}
