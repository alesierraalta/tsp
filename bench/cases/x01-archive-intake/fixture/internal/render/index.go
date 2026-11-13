package render

import (
	"fmt"
	"strings"
	"time"

	"example.com/archive/internal/store"
	"example.com/archive/internal/textutil"
)

// Index is the document index: the counts above a table of every object the caller may read. The table
// is built by Table, so a title carrying markup is text on the page.
func Index(records []store.Record, now time.Time) string {
	var total int64
	kinds := map[string]int{}
	for _, record := range records {
		total += record.Bytes
		kinds[record.Kind]++
	}
	var b strings.Builder
	b.WriteString("<section class=\"index\">\n")
	b.WriteString(Heading(1, "Archive"))
	b.WriteString(Paragraph(fmt.Sprintf("%d objects, %d bytes, %d pages", len(records), total, countPages(records))))
	b.WriteString(Paragraph("generated " + Stamp(now)))
	rows := make([][]string, 0, len(records))
	for _, record := range records {
		rows = append(rows, []string{
			record.Name,
			record.Label(),
			record.Kind,
			record.Tenant,
			fmt.Sprintf("%d", record.Pages),
			fmt.Sprintf("%d", record.Bytes),
			Stamp(record.CommittedAt()),
		})
	}
	b.WriteString(Table([]string{"name", "title", "kind", "tenant", "pages", "bytes", "committed"}, rows))
	b.WriteString("</section>\n")
	return Page("Archive", b.String())
}

// Object is one object's page: its metadata, its tags, and the text the pipeline extracted from it.
// The extracted text is written as preformatted, escaped text.
func Object(record store.Record, text string, now time.Time) string {
	var b strings.Builder
	b.WriteString("<article class=\"object\">\n")
	b.WriteString(Heading(1, record.Label()))
	b.WriteString(DefinitionList([][2]string{
		{"name", record.Name},
		{"id", record.ID},
		{"tenant", record.Tenant},
		{"kind", record.Kind},
		{"pages", fmt.Sprintf("%d", record.Pages)},
		{"bytes", fmt.Sprintf("%d", record.Bytes)},
		{"digest", record.Digest},
		{"batch", record.Batch},
		{"created", record.Created},
		{"committed", Stamp(record.CommittedAt())},
		{"rendered", Stamp(now)},
	}))
	if record.Source != "" {
		b.WriteString("<h2>Provenance</h2>\n")
		b.WriteString("<p>" + string(Link(record.Source, record.Source)) + "</p>\n")
	}
	b.WriteString("<h2>Tags</h2>\n<ul>\n")
	for _, tag := range record.Tags {
		b.WriteString("<li>" + Escape(tag) + "</li>\n")
	}
	b.WriteString("</ul>\n")
	b.WriteString("<h2>Text</h2>\n")
	b.WriteString(string(Code(text)))
	b.WriteString("</article>\n")
	return Page(record.Label(), b.String())
}

// Preview is the short object view the listing links to: the summary line of the extracted text and
// the metadata an operator scans for.
func Preview(record store.Record, text string) string {
	var b strings.Builder
	b.WriteString("<section class=\"preview\">\n")
	b.WriteString(Heading(2, record.Label()))
	b.WriteString(Paragraph(SummaryLine(text)))
	b.WriteString(DefinitionList([][2]string{
		{"kind", record.Kind},
		{"pages", fmt.Sprintf("%d", record.Pages)},
		{"bytes", fmt.Sprintf("%d", record.Bytes)},
	}))
	b.WriteString("</section>\n")
	return Page("Preview: "+record.Label(), b.String())
}

// SummaryLine is the first non-empty line of an extracted text, shortened for a listing row.
func SummaryLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		return textutil.Label(strings.TrimSpace(line))
	}
	return "(no text)"
}

// countPages sums the page counts of a record set.
func countPages(records []store.Record) int {
	total := 0
	for _, record := range records {
		total += record.Pages
	}
	return total
}

// NotFound is the page a browser gets for an object the tenant does not carry.
func NotFound(what string) string {
	return Page("Not found", "<section class=\"empty\">\n"+Heading(1, "Not found")+Paragraph(what)+"</section>\n")
}
