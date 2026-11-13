package archive

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Errors a manifest reports.
var (
	ErrManifest = errors.New("archive: malformed manifest")
)

// Manifest is the key=value description a stream may carry at its root. The importer reads it to
// learn the stream's declared totals; it never reads it to learn where a file goes, because where a
// file goes is decided by the path rule.
type Manifest struct {
	Stream  string
	Entries map[string]int64
}

// ParseManifest reads a manifest body. Every line is "key=value"; a line without a separator, or a
// value that is not a non-negative integer, is refused with its line number.
func ParseManifest(body []byte) (Manifest, error) {
	manifest := Manifest{Entries: map[string]int64{}}
	for i, raw := range strings.Split(string(body), "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return Manifest{}, fmt.Errorf("%w: line %d has no separator", ErrManifest, i+1)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			return Manifest{}, fmt.Errorf("%w: line %d has no key", ErrManifest, i+1)
		}
		switch key {
		case "stream":
			manifest.Stream = value
		case "records":
			n, err := strconv.ParseInt(value, 10, 64)
			if err != nil || n < 0 {
				return Manifest{}, fmt.Errorf("%w: line %d: records=%q", ErrManifest, i+1, value)
			}
			manifest.Entries["records"] = n
		default:
			n, err := strconv.ParseInt(value, 10, 64)
			if err != nil || n < 0 {
				return Manifest{}, fmt.Errorf("%w: line %d: %s=%q", ErrManifest, i+1, key, value)
			}
			manifest.Entries[key] = n
		}
	}
	if manifest.Stream == "" {
		return Manifest{}, fmt.Errorf("%w: no stream name", ErrManifest)
	}
	return manifest, nil
}

// Keys lists the manifest's declared keys, sorted.
func (m Manifest) Keys() []string {
	out := make([]string, 0, len(m.Entries))
	for key := range m.Entries {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// Declared returns the declared count of one key and whether it was declared at all. A key the
// manifest does not carry reads as zero, which the reconciler treats as "nothing promised" rather
// than "nothing arrived".
func (m Manifest) Declared(key string) (int64, bool) {
	n, ok := m.Entries[key]
	return n, ok
}

// FindManifest returns the entry the stream carries at its root as the manifest, if any.
func FindManifest(entries []Entry) (Manifest, bool, error) {
	for _, entry := range entries {
		if entry.Name != "manifest.txt" {
			continue
		}
		manifest, err := ParseManifest(entry.Body)
		if err != nil {
			return Manifest{}, true, err
		}
		return manifest, true, nil
	}
	return Manifest{}, false, nil
}
