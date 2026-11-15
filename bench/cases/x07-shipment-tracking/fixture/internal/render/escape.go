// Package render is where the service builds markup. Every value that reaches a page passes through
// Escape or Attr first, including the values this package interpolates itself — a partner's display
// name, a shipment's facility, a stored note — so a name registered as `<b>x</b>` is text on the page
// and never markup. No other package writes an HTML tag, which is what makes the rule checkable.
package render

import (
	"html"
	"strconv"
	"strings"
)

// Escape makes a value safe as element content. The five characters that can end an element, open a
// new one or carry an attribute are replaced by entities, so the value cannot leave the text node it
// is written into. It is html.EscapeString, named here so every call site says what it means.
func Escape(value string) string { return html.EscapeString(value) }

// Attr makes a value safe as a quoted attribute value. It escapes the same characters as Escape and
// additionally quotes the value, so a value carrying a double quote cannot end the attribute: the
// page writes `title="<escaped>"` and the escaped value is the whole value.
func Attr(value string) string { return `"` + html.EscapeString(value) + `"` }

// Class makes a value safe as a single class name: a value that is not a lower-case class is replaced
// rather than escaped, because a class is the page's vocabulary and not data.
func Class(value string) string {
	if value == "" {
		return "none"
	}
	var b strings.Builder
	for i := 0; i < len(value); i++ {
		c := value[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('-')
	}
	return b.String()
}

// Integer renders an integer for a page. It is a decimal rendering of a value that is already a
// number, never a string the page echoes.
func Integer(value int64) string { return strconv.FormatInt(value, 10) }

// Count renders a count with a thousands separator, so a page shows 12,345 rather than 12345.
func Count(value int) string {
	digits := strconv.Itoa(value)
	sign := ""
	if strings.HasPrefix(digits, "-") {
		sign = "-"
		digits = digits[1:]
	}
	var parts []string
	for len(digits) > 3 {
		parts = append([]string{digits[len(digits)-3:]}, parts...)
		digits = digits[:len(digits)-3]
	}
	parts = append([]string{digits}, parts...)
	return sign + strings.Join(parts, ",")
}

// Note renders a stored note as element content: escaped, with its line breaks kept as text line
// breaks rather than as markup. The note was refused at ingest if it carried a control character, so
// the only break it can carry is a newline this function turns into a `<br>` of its own.
func Note(value string) string {
	escaped := Escape(strings.ReplaceAll(value, "\r\n", "\n"))
	return strings.ReplaceAll(escaped, "\n", "<br>")
}
