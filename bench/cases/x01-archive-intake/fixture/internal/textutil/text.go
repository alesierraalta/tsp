// Package textutil holds the small text rules the archive applies in more than one place: how a
// label becomes a slug, how a title is trimmed for a page, how tags are split.
package textutil

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxLabel is the longest label the pages render in full. A longer label is shortened with an
// ellipsis at a rune boundary, never in the middle of a multi-byte character.
const MaxLabel = 80

// Slug folds a title into a key the search index and the export file names use. Only ASCII letters
// and digits survive; every run of other characters becomes one dash.
func Slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.TrimSpace(s) {
		switch {
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
			dash = false
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// Label shortens a value to MaxLabel runes for a page or a log line. It counts runes, so a title in a
// multi-byte script is not cut mid-character and rendered as a replacement glyph.
func Label(s string) string {
	trimmed := strings.TrimSpace(s)
	if utf8.RuneCountInString(trimmed) <= MaxLabel {
		return trimmed
	}
	count := 0
	for i := range trimmed {
		if count == MaxLabel-1 {
			return trimmed[:i] + "\u2026"
		}
		count++
	}
	return trimmed
}

// Collapse turns every run of whitespace into one space and trims the ends. A manifest title that
// carries a newline is one line on the page.
func Collapse(s string) string { return strings.Join(strings.Fields(s), " ") }

// SplitTags splits a comma-separated tag list, dropping blanks and repeats while keeping the order
// the author wrote. Tags are case-sensitive labels, not ids.
func SplitTags(s string) []string {
	var out []string
	seen := map[string]bool{}
	for _, part := range strings.Split(s, ",") {
		tag := strings.TrimSpace(part)
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		out = append(out, tag)
	}
	return out
}

// JoinTags renders tags the way the export writes them: comma and space, in stored order.
func JoinTags(tags []string) string { return strings.Join(tags, ", ") }

// Printable reports whether s carries no control characters other than tab and newline. The store
// refuses a name or a title that carries one, because it would break a CSV cell or a header line.
func Printable(s string) bool {
	for _, r := range s {
		if r == '\t' || r == '\n' || r == '\r' {
			continue
		}
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// PrefixFold reports whether s starts with prefix when both are lowercased with strings.ToLower.
// It is the comparison the search filter uses for a query prefix and nothing else: it is not an
// identity check, and the tenant and scope comparisons never use it.
func PrefixFold(s, prefix string) bool {
	return strings.HasPrefix(strings.ToLower(s), strings.ToLower(prefix))
}

// Sentence joins clauses into one description line for a page or an audit record.
func Sentence(clauses ...string) string {
	var kept []string
	for _, c := range clauses {
		if trimmed := strings.TrimSpace(c); trimmed != "" {
			kept = append(kept, strings.TrimSuffix(trimmed, "."))
		}
	}
	return strings.Join(kept, "; ")
}
