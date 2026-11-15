// Package textutil renders the labels the billing console prints: titles, slugs, truncated notes and
// the one-line summary of a row of numbers.
package textutil

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
)

// Title renders a name as a title: the first letter of every word upper-cased and the rest kept as
// written, so "acme logistics" becomes "Acme Logistics" and "Globex GmbH" keeps its own capitals.
func Title(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	var b strings.Builder
	start := true
	for _, r := range value {
		switch {
		case start && unicode.IsLetter(r):
			b.WriteRune(unicode.ToUpper(r))
			start = false
		case unicode.IsSpace(r):
			b.WriteRune(r)
			start = true
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Slug renders a name as the key a plan code is built from: lower-case letters, digits and single
// hyphens.
func Slug(value string) string {
	var b strings.Builder
	lastHyphen := true
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastHyphen = false
		case !lastHyphen:
			b.WriteByte('-')
			lastHyphen = true
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}

// Shorten truncates a value to at most width runes, ending it with an ellipsis when it was cut. A
// width of zero or less returns the empty string.
func Shorten(value string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	if width <= 1 {
		return string(runes[:width])
	}
	return string(runes[:width-1]) + "…"
}

// Plural renders a count with its noun, adding an s when the count is not one.
func Plural(count int, noun string) string {
	if count == 1 {
		return fmt.Sprintf("%d %s", count, noun)
	}
	return fmt.Sprintf("%d %ss", count, noun)
}

// JoinNumbers renders the keys and values of a count map as one line, in key order, which is what the
// console prints beside a plan.
func JoinNumbers(values map[string]int64) string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", key, values[key]))
	}
	return strings.Join(parts, " ")
}

// Camel joins two words as the name of a field or a column.
func Camel(left, right string) string {
	if left == "" {
		return right
	}
	if right == "" {
		return left
	}
	return left + "-" + right
}

// SplitList splits a comma-separated list and drops empty entries.
func SplitList(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// Age renders a duration as the number of whole days it covers, for a listing that spans cycles.
func Age(days int64) string {
	switch {
	case days <= 0:
		return "today"
	case days == 1:
		return "1 day"
	case days < 30:
		return fmt.Sprintf("%d days", days)
	case days < 60:
		return "1 month"
	case days < 365:
		return fmt.Sprintf("%d months", days/30)
	}
	return fmt.Sprintf("%d years", days/365)
}
