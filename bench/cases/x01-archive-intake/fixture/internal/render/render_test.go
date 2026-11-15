package render

import (
	"strings"
	"testing"
	"time"

	"example.com/archive/internal/intake"
	"example.com/archive/internal/manifest"
	"example.com/archive/internal/store"
)

func record() store.Record {
	return store.Record{
		ID:      "doc-00112233445566778899aabb",
		Tenant:  "acme",
		Name:    "scan-01",
		Kind:    "scan",
		Title:   "Letter <from> the assessor",
		Tags:    []string{"intake", "a<b"},
		Source:  "https://scanner.example/scan-01.pdf",
		Batch:   "bat-00112233445566778899aabb",
		Entry:   "scan-01",
		Bytes:   120,
		Digest:  strings.Repeat("ab", 32),
		Pages:   2,
		Created: "2024-04-30T09:00:00Z",
		Commit:  "2024-05-01T12:00:00Z",
	}
}

func TestEscapeCoversMarkup(t *testing.T) {
	got := Escape(`<a href="x">'&'</a>`)
	for _, want := range []string{"&lt;a", "&gt;", "&#34;", "&#39;", "&amp;"} {
		if !strings.Contains(got, want) {
			t.Fatalf("Escape = %q, want %q", got, want)
		}
	}
	if Attr("a\nb") != "a b" {
		t.Fatalf("Attr = %q", Attr("a\nb"))
	}
}

func TestObjectPageEscapesEveryField(t *testing.T) {
	page := Object(record(), "[page 001] text <b>", time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC))
	if strings.Contains(page, "Letter <from>") || strings.Contains(page, "a<b") || strings.Contains(page, "text <b>") {
		t.Fatalf("a value was rendered as markup:\n%s", page)
	}
	for _, want := range []string{"Letter &lt;from&gt; the assessor", "a&lt;b", "text &lt;b&gt;"} {
		if !strings.Contains(page, want) {
			t.Fatalf("page does not carry %q", want)
		}
	}
}

func TestIndexRendersATable(t *testing.T) {
	page := Index([]store.Record{record()}, time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC))
	if !strings.Contains(page, "<table>") || !strings.Contains(page, "scan-01") {
		t.Fatalf("page = %s", page)
	}
	if strings.Contains(page, "<a href=\"https://scanner.example/scan-01.pdf\"><script") {
		t.Fatalf("page carries an unescaped link")
	}
}

func TestBatchReportEscapesManifestValues(t *testing.T) {
	m := manifest.Manifest{
		ID:      "bat-00112233445566778899aabb",
		Tenant:  "acme",
		Created: "2024-04-30T09:00:00Z",
		Entries: []manifest.Entry{
			{Name: "scan-01", Kind: manifest.KindScan, Title: "<b>title</b>", Pages: 1,
				Source: "https://scanner.example/scan-01.pdf", Note: "plain note"},
			{Name: "listing.csv", Kind: manifest.KindIndex, Note: "12 rows, 4 columns"},
		},
	}
	result := intake.Result{Batch: m.ID, Tenant: m.Tenant, Committed: true,
		Entries: []intake.EntryResult{{Name: "scan-01", Object: record().ID, Kind: "scan", Pages: 1, Bytes: 120}}}
	page := BatchReport(m, result, []store.Record{record()}, time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC))
	if strings.Contains(page, "<b>title</b>") {
		t.Fatalf("the title was rendered as markup:\n%s", page)
	}
	if !strings.Contains(page, "&lt;b&gt;title&lt;/b&gt;") {
		t.Fatalf("the escaped title is missing:\n%s", page)
	}
	for _, want := range []string{"12 rows, 4 columns", "plain note", "scanner.example/scan-01.pdf"} {
		if !strings.Contains(page, want) {
			t.Fatalf("report does not carry %q", want)
		}
	}
}

func TestResumeReportShowsStagedEntries(t *testing.T) {
	state := intake.ResumeState{Batch: "bat-00112233445566778899aabb", Tenant: "acme", Used: 42,
		Entries: []intake.ResumeEntry{{Name: "scan-01", Kind: "scan", Staged: true}}}
	page := ResumeReport(state, time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC))
	for _, want := range []string{"scan-01", "staged, not committed", "42"} {
		if !strings.Contains(page, want) {
			t.Fatalf("page does not carry %q:\n%s", want, page)
		}
	}
}

func TestSummaryLineAndPage(t *testing.T) {
	if got := SummaryLine("\n\n[page 001] first line\nsecond"); got != "[page 001] first line" {
		t.Fatalf("summary = %q", got)
	}
	if got := SummaryLine(""); got != "(no text)" {
		t.Fatalf("empty summary = %q", got)
	}
	page := Page("<title>", "<p>body</p>")
	if !strings.Contains(page, "<title>&lt;title&gt;</title>") {
		t.Fatalf("page title = %s", page)
	}
}
