package render

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"example.com/archive/internal/audit"
	"example.com/archive/internal/config"
	"example.com/archive/internal/store"
)

// Console is the operator console's first page: the configuration the process runs with, the store's
// summary, the staged batches and the returning link a deployment prints in its own operations notes.
func Console(cfg config.Config, stats store.Stats, staged []string, trail *audit.Trail, now time.Time) string {
	var b strings.Builder
	b.WriteString("<section class=\"console\">\n")
	b.WriteString(Heading(1, "Operator console"))
	b.WriteString(Paragraph("generated " + Stamp(now)))
	b.WriteString(Heading(2, "Running with"))
	settings := make([][]string, 0, len(config.Variables()))
	for _, variable := range config.Variables() {
		value := variable.Default
		if variable.Required {
			value = "set"
		}
		settings = append(settings, []string{variable.Name, value, variable.Meaning})
	}
	b.WriteString(Table([]string{"variable", "value", "meaning"}, settings))
	b.WriteString(Heading(2, "Store"))
	counts := make([][]string, 0, len(stats.Kinds))
	for _, kind := range sorted(stats.Kinds) {
		counts = append(counts, []string{kind, strconv.Itoa(stats.Kinds[kind])})
	}
	b.WriteString(DefinitionList([][2]string{
		{"objects", strconv.Itoa(stats.Objects)},
		{"bytes", strconv.FormatInt(stats.Bytes, 10)},
		{"pages", strconv.Itoa(stats.Pages)},
		{"blobs", strconv.Itoa(stats.Blobs)},
		{"retention", fmt.Sprintf("%d UTC days", cfg.RetentionDays)},
		{"tenant list", cfg.TenantList()},
	}))
	if len(counts) > 0 {
		b.WriteString(Table([]string{"kind", "objects"}, counts))
	}
	b.WriteString(Heading(2, "Staged batches"))
	if len(staged) == 0 {
		b.WriteString(Paragraph("no batch is staged"))
	} else {
		rows := make([][]string, 0, len(staged))
		for _, id := range staged {
			rows = append(rows, []string{id})
		}
		b.WriteString(Table([]string{"batch"}, rows))
	}
	if trail != nil {
		b.WriteString(Heading(2, "Audit"))
		b.WriteString(Paragraph(trail.Summarize()))
	}
	b.WriteString("</section>\n")
	return Page("Operator console", b.String())
}

// TrailTable renders the audit page's table. A detail is escaped like every other cell.
func TrailTable(entries []audit.Entry) string {
	rows := make([][]string, 0, len(entries))
	for _, entry := range entries {
		rows = append(rows, []string{
			entry.At,
			entry.Subject,
			entry.Tenant,
			entry.Action,
			entry.Target,
			entry.Outcome,
			entry.Detail,
		})
	}
	return Table([]string{"at", "subject", "tenant", "action", "target", "outcome", "detail"}, rows)
}

// PolicyTable renders an action/scope table, which the console prints under "who may do what".
func PolicyTable(actions [][2]string) string {
	rows := make([][]string, 0, len(actions))
	for _, action := range actions {
		rows = append(rows, []string{action[0], action[1]})
	}
	return Table([]string{"action", "scopes required"}, rows)
}

// LineList renders a list of plain lines as preformatted, escaped text. It is how the console prints a
// report whose shape is already a text table.
func LineList(lines []string) string {
	return string(Code(strings.Join(lines, "\n")))
}

// sorted returns the keys of a count map in sorted order.
func sorted(counts map[string]int) []string {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
