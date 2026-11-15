package render

import (
	"fmt"
	"strings"
	"time"

	"example.com/archive/internal/intake"
	"example.com/archive/internal/manifest"
	"example.com/archive/internal/store"
)

// BatchReport is the page an operator reads after an import: what the batch declared, what the
// pipeline did with each entry, and the provenance of every object it committed.
//
// Every value a manifest carries is text the sender wrote, so the page escapes it. The one exception
// is an index entry's note: an index is the row-and-column listing the scanner writes itself, and its
// note is preformatted, so the report keeps its line breaks instead of escaping them into a paragraph.
func BatchReport(m manifest.Manifest, result intake.Result, committed []store.Record, now time.Time) string {
	var b strings.Builder
	b.WriteString("<section class=\"batch\">\n")
	b.WriteString(Heading(1, "Import "+m.ID))
	b.WriteString(Paragraph(result.Summary()))
	b.WriteString(DefinitionList([][2]string{
		{"batch", m.ID},
		{"tenant", m.Tenant},
		{"operator", m.Operator},
		{"created", m.Created},
		{"entries", fmt.Sprintf("%d", len(m.Entries))},
		{"pages", fmt.Sprintf("%d", m.Pages())},
		{"bytes", fmt.Sprintf("%d", m.TotalBytes())},
		{"rendered", Stamp(now)},
	}))
	b.WriteString(Heading(2, "Entries"))
	rows := make([][]string, 0, len(m.Entries))
	for _, e := range m.Entries {
		rows = append(rows, []string{
			e.Name,
			e.Label(),
			e.KindOf(),
			fmt.Sprintf("%d", e.Pages),
			fmt.Sprintf("%d", e.Bytes),
			e.DigestShort(),
			manifest.SourceClass(e.Source),
			e.Describe(),
		})
	}
	b.WriteString(Table([]string{"name", "title", "kind", "pages", "bytes", "digest", "source", "describe"}, rows))
	b.WriteString(Heading(2, "Provenance"))
	for _, e := range m.Entries {
		b.WriteString("<div class=\"entry\">\n")
		b.WriteString(Paragraph(e.Name + " from " + manifest.SourceLabel(e.Source)))
		if e.Source != "" {
			b.WriteString("<p class=\"source\">" + string(Link(e.Source, manifest.SourceLabel(e.Source))) + "</p>\n")
		}
		switch {
		case e.KindOf() == manifest.KindIndex && e.Note != "":
			// An index note is the listing header the scanner writes, one line per column set.
			b.WriteString("<pre class=\"index-note\">" + Escape(e.Note) + "</pre>\n")
		case e.Note != "":
			b.WriteString(Paragraph(e.Note))
		}
		b.WriteString("</div>\n")
	}
	if len(committed) > 0 {
		b.WriteString(Heading(2, "Committed objects"))
		objectRows := make([][]Cell, 0, len(committed))
		for _, record := range committed {
			objectRows = append(objectRows, []Cell{
				Link("/api/documents/"+record.ID, record.Label()),
				Cell(Escape(record.Entry)),
				Cell(Escape(record.Kind)),
				Cell(Escape(fmt.Sprintf("%d", record.Pages))),
				Cell(Escape(fmt.Sprintf("%d", record.Bytes))),
			})
		}
		b.WriteString(TableCells([]string{"object", "entry", "kind", "pages", "bytes"}, objectRows))
	}
	b.WriteString("</section>\n")
	return Page("Import "+m.ID, b.String())
}

// ResumeReport is the page the admin console shows for a batch that is not committed: which entries
// are staged, which committed, and how much of the quota the tree spends.
func ResumeReport(state intake.ResumeState, now time.Time) string {
	var b strings.Builder
	b.WriteString("<section class=\"resume\">\n")
	b.WriteString(Heading(1, "Batch "+state.Batch))
	status := "staged, not committed"
	if state.Committed {
		status = "committed"
	}
	b.WriteString(DefinitionList([][2]string{
		{"batch", state.Batch},
		{"tenant", state.Tenant},
		{"state", status},
		{"staged bytes", fmt.Sprintf("%d", state.Used)},
		{"rendered", Stamp(now)},
	}))
	rows := make([][]string, 0, len(state.Entries))
	for _, entry := range state.Entries {
		committed := "no"
		if entry.Committed {
			committed = entry.Object
		}
		rows = append(rows, []string{
			entry.Name,
			entry.Kind,
			fmt.Sprintf("%d", entry.Pages),
			yesNo(entry.Staged),
			committed,
		})
	}
	b.WriteString(Table([]string{"entry", "kind", "pages", "staged", "object"}, rows))
	b.WriteString("</section>\n")
	return Page("Batch "+state.Batch, b.String())
}

// yesNo renders a flag as the word a page prints.
func yesNo(flag bool) string {
	if flag {
		return "yes"
	}
	return "no"
}
