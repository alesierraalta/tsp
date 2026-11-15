package export

import (
	"example.com/archive/internal/clock"
	"fmt"
	"strings"
	"time"
)

// Cell renders one CSV cell as text. A leading =, +, -, @ or a tab makes a spreadsheet evaluate the
// cell, so those are prefixed with an apostrophe; anything else is kept as it is.
func Cell(value string) string {
	if value == "" {
		return ""
	}
	switch value[0] {
	case '=', '+', '-', '@', '\t', '\r':
		return "'" + value
	}
	return value
}

// Filename is the name the export is served under: the tenant, the day and the extension, with every
// character a header cannot carry replaced.
func Filename(tenant string, now time.Time) string {
	day := clock.Day(now).Format("20060102")
	clean := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, tenant)
	return fmt.Sprintf("archive-%s-%s.csv", clean, day)
}
