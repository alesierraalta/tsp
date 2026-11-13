package textutil

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Bytes renders a size the way the operator pages print one: an exact count below a kibibyte, and one
// decimal above it. The count is always the last thing in the line, so a page can be compared by eye.
func Bytes(n int64) string {
	switch {
	case n < 0:
		return "unknown"
	case n < 1<<10:
		return strconv.FormatInt(n, 10) + " B"
	case n < 1<<20:
		return fmt.Sprintf("%.1f KiB", float64(n)/(1<<10))
	case n < 1<<30:
		return fmt.Sprintf("%.1f MiB", float64(n)/(1<<20))
	default:
		return fmt.Sprintf("%.1f GiB", float64(n)/(1<<30))
	}
}

// Duration renders a span the way a batch report prints it: whole seconds below a minute, minutes and
// seconds above it, and an exact millisecond count below a second.
func Duration(d time.Duration) string {
	switch {
	case d < 0:
		return "unknown"
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	case d < time.Minute:
		return fmt.Sprintf("%.1fs", d.Seconds())
	default:
		minutes := int(d / time.Minute)
		seconds := int((d % time.Minute) / time.Second)
		return fmt.Sprintf("%dm%02ds", minutes, seconds)
	}
}

// Rate renders a byte rate, or "unknown" when the span is zero, so a page never divides by zero.
func Rate(bytes int64, span time.Duration) string {
	if span <= 0 || bytes < 0 {
		return "unknown"
	}
	return Bytes(int64(float64(bytes)/span.Seconds())) + "/s"
}

// Percent renders a ratio as a whole percentage, clamped to the 0..100 range the pages print.
func Percent(part, total int64) string {
	if total <= 0 || part < 0 {
		return "0%"
	}
	if part > total {
		part = total
	}
	return strconv.FormatInt(part*100/total, 10) + "%"
}

// PadRight pads a value with spaces to a rune width, never cutting it: a value longer than the width is
// returned whole, so a table column widens rather than loses text.
func PadRight(value string, width int) string {
	count := len([]rune(value))
	if count >= width {
		return value
	}
	return value + strings.Repeat(" ", width-count)
}

// PadLeft pads a value on the left, which is how the pages align a count column.
func PadLeft(value string, width int) string {
	count := len([]rune(value))
	if count >= width {
		return value
	}
	return strings.Repeat(" ", width-count) + value
}

// Plural renders a count with its noun, choosing the singular form for one.
func Plural(n int, singular, plural string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", singular)
	}
	return fmt.Sprintf("%d %s", n, plural)
}

// QuoteList renders a list as "a, b and c", which is how a report names the fields it refused.
func QuoteList(values []string) string {
	switch len(values) {
	case 0:
		return ""
	case 1:
		return values[0]
	case 2:
		return values[0] + " and " + values[1]
	default:
		return strings.Join(values[:len(values)-1], ", ") + " and " + values[len(values)-1]
	}
}
