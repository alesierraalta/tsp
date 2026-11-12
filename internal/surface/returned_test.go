package surface

import (
	"strings"
	"testing"
)

// The lexer fixes the returned rows depend on, pinned so they are not undone by a later edit to the token
// stream: a template interpolation is a group, and a definition does not continue the statement above it.

// A `${...}` inside a template is a group of its own: without that, the brace closing the interpolation closes
// the function's block, the return is split in two, and the markup it builds is never read.
func TestAJavaScriptTemplateInterpolationDoesNotEndTheStatement(t *testing.T) {
	body := "function escapeAttr(value) {\n  return String(value).replace(/[&<>]/g, () => '&lt;');\n}\n\nexport function attr(name, value) {\n  return `${name}=\"${escapeAttr(value)}\"`;\n}\n"
	dir := writeTree(t, map[string]string{"render.js": body})
	report, err := probe(t, dir, "render.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rowsOf(report, "attr") {
		if row.Sink == SinkReturnedMarkup && row.SinkLine == lineOf(t, body, "return `${name}") {
			return
		}
	}
	t.Fatalf("rows = %+v, want the returned markup row on the attr return", report.Rows)
}

// A Python `def` on the very next line starts a new statement. Reading it as a continuation of the `return`
// above hides the query that return built.
func TestAPythonDefinitionOnTheNextLineDoesNotContinueTheReturn(t *testing.T) {
	body := `import sqlite3


def lookup(db, name):
    return f"SELECT * FROM users WHERE name = '{name}'"
def close(db):
    db.close()
`
	dir := writeTree(t, map[string]string{"query.py": body})
	report, err := probe(t, dir, "query.py")
	if err != nil {
		t.Fatal(err)
	}
	rows := rowsOf(report, "lookup")
	if len(rows) != 1 || rows[0].Sink != SinkReturnedQuery {
		t.Fatalf("lookup rows = %+v, want one %q row", rows, SinkReturnedQuery)
	}
	if rows[0].SinkLine != lineOf(t, body, "return f\"SELECT") || rows[0].Guarded {
		t.Errorf("row = %+v, want the unguarded query at the return line", rows[0])
	}
	if !strings.Contains(rows[0].SinkCall, "SELECT") {
		t.Errorf("sink call = %q, want the statement the literal carries", rows[0].SinkCall)
	}
}

// Prose that happens to contain a SQL word is not a query: "anywhere else" is not a WHERE clause, so a row on it
// is a probe nobody can break.
func TestAReturnedPromptWithProseIsNotAQuery(t *testing.T) {
	body := `package fixture

import "fmt"

func Prompt(findingsPath string) string {
	return fmt.Sprintf("Read the findings at %s and record a pin anywhere else is a pin the run never reports.", findingsPath)
}
`
	dir := writeTree(t, map[string]string{"prompt.go": body})
	report, err := probe(t, dir, "prompt.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range report.Rows {
		if row.Sink == SinkReturnedQuery {
			t.Fatalf("row = %+v, want no returned query: the literal carries prose, not a statement", row)
		}
	}
}

// Bracketed prose is not markup: `<that file>` has a space in the tag name, which no tag carries, while a tag
// with an attribute keeps its row.
func TestBracketedProseIsNotReturnedMarkup(t *testing.T) {
	prose := "export function note(value) {\n" +
		"  return `submit it with tsp feedback --file <that file> and nothing else ${value}`;\n" +
		"}\n"
	dir := writeTree(t, map[string]string{"note.js": prose})
	report, err := probe(t, dir, "note.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range report.Rows {
		if row.Sink == SinkReturnedMarkup {
			t.Fatalf("row = %+v, want no returned markup: the brackets hold prose", row)
		}
	}

	tagged := "export function span(value) {\n" +
		"  return `<span class=\"muted\">${value}</span>`;\n" +
		"}\n"
	dir = writeTree(t, map[string]string{"span.js": tagged})
	report, err = probe(t, dir, "span.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range report.Rows {
		if row.Sink == SinkReturnedMarkup {
			return
		}
	}
	t.Fatalf("rows = %+v, want the returned markup row for a real tag", report.Rows)
}
