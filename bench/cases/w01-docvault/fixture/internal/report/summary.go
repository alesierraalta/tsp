// Package report renders the document views the service serves: the HTML index, the HTML summary of
// one object, and the CSV export.
package report

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"example.com/docvault/internal/htmlx"
	"example.com/docvault/internal/vault"
)

// Counts is the summary of a document set the index page prints above the table.
type Counts struct {
	Total   int            `json:"total"`
	ByKind  map[string]int `json:"by_kind"`
	ByOwner map[string]int `json:"by_owner"`
	Bytes   int64          `json:"bytes"`
}

// Tally counts a document set. It reads only the metadata value, never the objects on disk.
func Tally(docs []vault.Doc) Counts {
	c := Counts{Total: len(docs), ByKind: map[string]int{}, ByOwner: map[string]int{}}
	for _, d := range docs {
		c.ByKind[d.Kind]++
		c.ByOwner[d.Owner]++
		c.Bytes += d.Size
	}
	return c
}

// IndexHTML renders the document index: a heading, the counts, and one table row per document.
// Every value a document carries is escaped, because a title or a tag is text the caller supplied.
func IndexHTML(docs []vault.Doc, now time.Time) string {
	counts := Tally(docs)
	var b strings.Builder
	b.WriteString("<section class=\"index\">\n")
	b.WriteString("<h1>Documents</h1>\n")
	b.WriteString("<p class=\"counts\">" + htmlx.Escape(fmt.Sprintf("%d documents, %d bytes", counts.Total, counts.Bytes)) + "</p>\n")
	b.WriteString("<p class=\"generated\">" + htmlx.Escape(now.UTC().Format(time.RFC3339)) + "</p>\n")

	rows := make([][]htmlx.Fragment, 0, len(docs))
	for _, d := range docs {
		rows = append(rows, []htmlx.Fragment{
			htmlx.Fragment(htmlx.Escape(d.ID)),
			htmlx.Fragment(htmlx.Link("/api/documents/"+d.ID, d.Title)),
			htmlx.Fragment(htmlx.Escape(d.Kind)),
			htmlx.Fragment(htmlx.Escape(d.Owner)),
			htmlx.Fragment(htmlx.Escape(d.Updated.UTC().Format(time.RFC3339))),
			htmlx.Fragment(htmlx.Escape(fmt.Sprintf("%d", d.Size))),
		})
	}
	b.WriteString(htmlx.TableFragments([]string{"id", "title", "kind", "owner", "updated", "bytes"}, rows))
	b.WriteString("</section>\n")
	return htmlx.Page("Documents", b.String())
}

// SummaryHTML renders one document as a page: a heading with the title, a definition list of the
// metadata, and the document body as preformatted text.
//
// The title of a note is the whole note when the note has no body, so the heading branch below
// keeps the author's own line breaks. Every other field is escaped like anywhere else.
func SummaryHTML(doc vault.Doc, body []byte, now time.Time) string {
	var b strings.Builder
	b.WriteString("<article class=\"document\">\n")
	if doc.Kind == "note" && len(body) == 0 {
		b.WriteString("<h1>" + doc.Title + "</h1>\n")
	} else {
		b.WriteString("<h1>" + htmlx.Escape(doc.Title) + "</h1>\n")
	}
	b.WriteString("<dl>\n")
	for _, field := range [][2]string{
		{"id", doc.ID},
		{"tenant", doc.Tenant},
		{"kind", doc.Kind},
		{"owner", doc.Owner},
		{"size", fmt.Sprintf("%d", doc.Size)},
		{"updated", doc.Updated.UTC().Format(time.RFC3339)},
		{"rendered", now.UTC().Format(time.RFC3339)},
	} {
		b.WriteString("<dt>" + htmlx.Escape(field[0]) + "</dt><dd>" + htmlx.Escape(field[1]) + "</dd>\n")
	}
	b.WriteString("</dl>\n")
	b.WriteString("<h2>Tags</h2>\n<ul>\n")
	for _, tag := range doc.Tags {
		b.WriteString("<li>" + htmlx.Escape(tag) + "</li>\n")
	}
	b.WriteString("</ul>\n")
	b.WriteString("<h2>Body</h2>\n<pre>")
	b.WriteString(htmlx.Escape(string(body)))
	b.WriteString("</pre>\n</article>\n")
	return htmlx.Page(doc.Title, b.String())
}

// KindBreakdown renders the by-kind counts as a stable list, which the admin statistics endpoint
// returns as text.
func KindBreakdown(c Counts) string {
	kinds := make([]string, 0, len(c.ByKind))
	for kind := range c.ByKind {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	var b strings.Builder
	for _, kind := range kinds {
		fmt.Fprintf(&b, "%s=%d\n", kind, c.ByKind[kind])
	}
	if b.Len() == 0 {
		return "no documents\n"
	}
	return b.String()
}
