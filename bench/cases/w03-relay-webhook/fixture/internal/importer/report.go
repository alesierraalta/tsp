package importer

import (
	"encoding/json"
	"sort"
	"strings"

	"example.com/relay/internal/archive"
)

// Summary is the text an operator reads after an import: the counts, the kinds and the written
// keys, in a stable order.
type Summary struct {
	Entries int
	Written int
	Records int
	Kinds   map[string]int
	Keys    []string
}

// Summarize reads a report and the entries it came from into the operator view.
func Summarize(report Report, entries []archive.Entry) Summary {
	summary := Summary{
		Entries: report.Entries,
		Written: report.Written,
		Records: report.Records,
		Kinds:   map[string]int{},
	}
	for _, entry := range entries {
		kind := archive.Kind(entry.Name)
		if kind == "" {
			kind = "(none)"
		}
		summary.Kinds[kind]++
	}
	for _, path := range report.Paths {
		if key, err := archive.Key(pathRoot(path), path); err == nil {
			summary.Keys = append(summary.Keys, key)
		}
	}
	sort.Strings(summary.Keys)
	return summary
}

// JSON renders the summary as the body the operator endpoint serves.
func (s Summary) JSON() ([]byte, error) {
	return json.Marshal(map[string]any{
		"entries": s.Entries,
		"written": s.Written,
		"records": s.Records,
		"kinds":   s.Kinds,
		"keys":    s.Keys,
	})
}

// Text renders the summary as one line per kind plus the written keys.
func (s Summary) Text() string {
	var b strings.Builder
	kindNames := make([]string, 0, len(s.Kinds))
	for kind := range s.Kinds {
		kindNames = append(kindNames, kind)
	}
	sort.Strings(kindNames)
	for _, kind := range kindNames {
		b.WriteString(kind)
		b.WriteString("=")
		b.WriteString(itoa(s.Kinds[kind]))
		b.WriteString("\n")
	}
	for _, key := range s.Keys {
		b.WriteString("wrote ")
		b.WriteString(key)
		b.WriteString("\n")
	}
	return b.String()
}

// pathRoot is the root a staged path belongs to: the directory above the entry's own kind. A path
// the pipeline wrote is always under the staging directory, so its root is the first segment the
// kind names.
func pathRoot(path string) string {
	slashed := strings.ReplaceAll(path, "\\", "/")
	cut := strings.Index(slashed, "/attachments/")
	if cut < 0 {
		cut = strings.Index(slashed, "/thumbnails/")
	}
	if cut < 0 {
		return "."
	}
	return slashed[:cut]
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
