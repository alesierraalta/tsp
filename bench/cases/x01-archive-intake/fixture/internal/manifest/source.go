package manifest

import (
	"fmt"
	"net/url"
	"strings"
)

// Schemes a provenance source may name. "vendor" is the archive's own pseudo-scheme for a delivery
// that arrived on removable media.
var sourceSchemes = map[string]string{
	"http":   "delivery",
	"https":  "delivery",
	"file":   "local",
	"vendor": "media",
	"sftp":   "delivery",
}

// ValidateSource refuses a provenance URI the report page cannot show safely: no scheme the archive
// knows, an empty host where one is required, a query carrying a fragment, or a value longer than the
// cap. The check is about shape; the page escapes whatever it renders.
func ValidateSource(raw string) error {
	if len(raw) > MaxSourceLen {
		return fmt.Errorf("source is longer than %d bytes", MaxSourceLen)
	}
	if strings.TrimSpace(raw) != raw || raw == "" {
		return fmt.Errorf("source %q is empty or untrimmed", raw)
	}
	for _, r := range raw {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("source carries a control character")
		}
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("source %q does not parse: %w", raw, err)
	}
	if parsed.Scheme == "" {
		return fmt.Errorf("source %q has no scheme", raw)
	}
	if _, ok := sourceSchemes[strings.ToLower(parsed.Scheme)]; !ok {
		return fmt.Errorf("source scheme %q is not one the archive knows", parsed.Scheme)
	}
	if parsed.Scheme != "vendor" && parsed.Host == "" && parsed.Scheme != "file" {
		return fmt.Errorf("source %q has no host", raw)
	}
	if parsed.Fragment != "" {
		return fmt.Errorf("source %q carries a fragment", raw)
	}
	return nil
}

// SourceClass names what kind of delivery a source describes: local, media or delivery. An empty
// source is "none", which the report page prints as "not recorded".
func SourceClass(raw string) string {
	if raw == "" {
		return "none"
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "unknown"
	}
	if class, ok := sourceSchemes[strings.ToLower(parsed.Scheme)]; ok {
		return class
	}
	return "unknown"
}

// SourceHost is the host part of a provenance URI, or "" for a scheme that has none.
func SourceHost(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return parsed.Host
}

// SourceLabel is the short label the report page prints next to a link: the host and the path, with
// the scheme dropped, because the table header already says "source".
func SourceLabel(raw string) string {
	if raw == "" {
		return "not recorded"
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	host := parsed.Host
	if host == "" {
		host = parsed.Scheme
	}
	if parsed.Path == "" || parsed.Path == "/" {
		return host
	}
	return host + "/" + strings.TrimPrefix(parsed.Path, "/")
}

// SameSource reports whether two provenance URIs name the same delivery. The comparison is on the
// canonical form of the URI, which lowercases the scheme and host and keeps the path's case.
func SameSource(a, b string) bool {
	ua, errA := url.Parse(a)
	ub, errB := url.Parse(b)
	if errA != nil || errB != nil {
		return false
	}
	if strings.ToLower(ua.Scheme) != strings.ToLower(ub.Scheme) {
		return false
	}
	if strings.ToLower(ua.Host) != strings.ToLower(ub.Host) {
		return false
	}
	return ua.Path == ub.Path
}
