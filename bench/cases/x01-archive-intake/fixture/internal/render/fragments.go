package render

import (
	"strings"
)

// Package render builds every HTML the archive serves: the document index, one object's page, the
// batch report and the operator console. Every value is escaped here, so a handler cannot decide on
// its own that a value is safe to write raw.

// Escape renders text for an HTML text node or a quoted attribute. Every value a manifest, a record or
// a page carries goes through it.
func Escape(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 8)
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		case '"':
			b.WriteString("&#34;")
		case '\'':
			b.WriteString("&#39;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Attr renders a value for a double-quoted attribute. It is Escape plus a refusal of the values an
// attribute cannot carry at all: a tab or a newline would end the attribute in some parsers.
func Attr(s string) string {
	cleaned := strings.Map(func(r rune) rune {
		switch r {
		case '\t', '\n', '\r':
			return ' '
		}
		return r
	}, s)
	return Escape(cleaned)
}

// Page wraps a rendered body in the archive's layout. title is escaped like any other text.
func Page(title, body string) string {
	var b strings.Builder
	b.WriteString("<!doctype html>\n<html lang=\"en\"><head><meta charset=\"utf-8\"><title>")
	b.WriteString(Escape(title))
	b.WriteString("</title></head>\n<body>\n")
	b.WriteString(body)
	b.WriteString("</body></html>\n")
	return b.String()
}
