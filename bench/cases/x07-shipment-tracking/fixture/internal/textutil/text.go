// Package textutil holds the small string rules the service applies in more than one place: the slug
// a report file is named by, the label an operator reads, a byte count a page prints, and the cut a
// table cell takes. Nothing here escapes markup; that is internal/render's job and it is the only
// place a value is interpolated into a page.
package textutil

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Slug turns a human name into the name a file may carry: ASCII lowercase letters and digits, with
// runs of anything else collapsed to one dash. A slug is never empty: a value with no letter or digit
// in it becomes "item".
func Slug(value string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(value) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			if dash && b.Len() > 0 {
				b.WriteByte('-')
			}
			dash = false
			b.WriteRune(r)
		case r < utf8.RuneSelf && unicode.IsSpace(r), r == '-', r == '_', r == '.', r == '/':
			dash = true
		default:
			// A rune outside the ASCII range is a separator, not a transliteration: the slug is a file
			// name, and two names that differ only by an accent must not become one file.
			dash = true
		}
	}
	if b.Len() == 0 {
		return "item"
	}
	return b.String()
}

// Label trims a display value and collapses its inner whitespace, so a column of a report lines up.
// The value keeps its case and its bytes; only spaces are touched.
func Label(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

// Clip cuts a value to at most n runes, on a rune boundary, and marks the cut with an ellipsis. A cut
// that would split a rune cuts before it, so a page never prints half a character.
func Clip(value string, n int) string {
	if n <= 0 {
		return ""
	}
	if utf8.RuneCountInString(value) <= n {
		return value
	}
	if n <= 1 {
		return "…"
	}
	var b strings.Builder
	count := 0
	for _, r := range value {
		if count == n-1 {
			break
		}
		b.WriteRune(r)
		count++
	}
	return b.String() + "…"
}

// Bytes prints a byte count the way an operator reads it: whole units with one fraction digit for
// kilo, mega and giga, and the exact count below a kilobyte.
func Bytes(n int64) string {
	if n < 0 {
		return "-" + Bytes(-n)
	}
	switch {
	case n < 1024:
		return strconv.FormatInt(n, 10) + " B"
	case n < 1024*1024:
		return oneDecimal(n, 1024) + " KiB"
	case n < 1024*1024*1024:
		return oneDecimal(n, 1024*1024) + " MiB"
	default:
		return oneDecimal(n, 1024*1024*1024) + " GiB"
	}
}

// oneDecimal renders n/unit with one truncated fraction digit, from integers only.
func oneDecimal(n, unit int64) string {
	whole := n / unit
	tenth := (n % unit) * 10 / unit
	return strconv.FormatInt(whole, 10) + "." + strconv.FormatInt(tenth, 10)
}

// Grams prints a weight in grams as kilograms with three fraction digits, which is the unit a
// carrier bills in.
func Grams(grams int64) string {
	if grams < 0 {
		return "-" + Grams(-grams)
	}
	return fmt.Sprintf("%d.%03d", grams/1000, grams%1000)
}

// Join joins non-empty parts with sep, skipping a part that is only whitespace.
func Join(sep string, parts ...string) string {
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, sep)
}

// HasPrefix reports whether value begins with prefix, on a byte boundary. It is the comparison the
// name rules use, and it is exact: there is no case folding and no normalization.
func HasPrefix(value, prefix string) bool { return strings.HasPrefix(value, prefix) }
