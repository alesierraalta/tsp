package manifest

import (
	"fmt"
	"strings"
)

// Rule is one validation rule the accept step applies to a manifest. The table is documentation and
// code at once: the admin page prints it, and the test that covers validation reads it, so a rule cannot
// be dropped without the printed contract changing.
type Rule struct {
	Field    string
	Rule     string
	Rejected string
}

// Rules lists the checks Validate applies, in the order it applies them.
func Rules() []Rule {
	return []Rule{
		{"tenant", "present and one of the deployment's tenants", "an empty tenant"},
		{"created", "RFC3339, stored back in UTC to the second", "an empty or unparsable instant"},
		{"entries", "between 1 and the deployment's entry cap", "no entries, or more than the cap"},
		{"entry.name", "one flat segment of a-z, 0-9, dash, underscore, dot", "a separator, a dot segment, a control character"},
		{"entry.name", "unique inside the batch", "a name the batch already carries"},
		{"entry.kind", "scan, page, sidecar or index", "any other kind"},
		{"entry.pages", "positive for a scan, zero for every other kind, at most the page cap", "a page count the kind cannot carry"},
		{"entry.bytes", "non-negative and at most the upload cap", "a negative or oversized size"},
		{"entry.digest", "lowercase SHA-256 hex when declared", "an uppercase, short or non-hex digest"},
		{"entry.title", "at most 200 bytes of printable text", "a control character or an oversized title"},
		{"entry.note", "at most 2000 bytes of printable text", "a control character or an oversized note"},
		{"entry.tags", "at most 8 trimmed tags of at most 32 bytes", "a repeated or oversized tag"},
		{"entry.source", "a URI with a known scheme, a host and no fragment", "an unknown scheme or a fragment"},
		{"batch.bytes", "the declared total at most the upload cap", "a total above the cap"},
		{"batch.pages", "the declared total at most the page cap", "a total above the cap"},
	}
}

// RuleFields lists the field names the rules cover, for the admin page's table header.
func RuleFields() []string {
	seen := map[string]bool{}
	var out []string
	for _, rule := range Rules() {
		if seen[rule.Field] {
			continue
		}
		seen[rule.Field] = true
		out = append(out, rule.Field)
	}
	return out
}

// KindRules lists what each kind must carry and what the pipeline does with it.
func KindRules() [][3]string {
	return [][3]string{
		{KindScan, "one container, one page index, at least one page", "verify the pages, extract their text, commit the container"},
		{KindPage, "one container only, no index", "commit the page image as an object of its own"},
		{KindSidecar, "one container holding the recognised text", "commit the text and index its tokens"},
		{KindIndex, "one container holding the row/column listing", "commit the listing and escape it on the report"},
	}
}

// DescribeKind renders one kind's rules as a sentence.
func DescribeKind(kind string) string {
	for _, row := range KindRules() {
		if row[0] == kind {
			return fmt.Sprintf("%s carries %s and the pipeline %s", row[0], row[1], row[2])
		}
	}
	return "unknown kind " + kind
}

// RequiredPages is the page count a kind must declare: a scan needs at least one, and every other kind
// declares none. The check is written once so a stage cannot disagree with the parser.
func RequiredPages(kind string, declared int) error {
	switch kind {
	case KindScan:
		if declared < 1 {
			return fmt.Errorf("a scan must declare at least one page")
		}
		return nil
	case KindPage, KindSidecar, KindIndex:
		if declared != 0 {
			return fmt.Errorf("a %s declares no pages", kind)
		}
		return nil
	default:
		return fmt.Errorf("unknown kind %q", kind)
	}
}

// NormalizeKind maps the spelling a sender used to the kind the service stores: case is folded, and the
// plural forms the scanner's exporter writes are accepted.
func NormalizeKind(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "scan", "scans":
		return KindScan
	case "page", "pages":
		return KindPage
	case "sidecar", "sidecars", "text":
		return KindSidecar
	case "index", "indexes", "indices":
		return KindIndex
	default:
		return ""
	}
}

// SummaryLine is the one-line label a listing prints for a manifest entry set.
func (m Manifest) SummaryLine() string {
	kinds := m.Kinds()
	parts := []string{fmt.Sprintf("%d entries", len(m.Entries))}
	for _, kind := range []string{KindScan, KindPage, KindSidecar, KindIndex} {
		if kinds[kind] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", kinds[kind], kind))
		}
	}
	parts = append(parts, fmt.Sprintf("%d pages", m.Pages()))
	return strings.Join(parts, ", ")
}
