package manifest

import (
	"example.com/archive/internal/checksum"
	"example.com/archive/internal/storepath"
	"example.com/archive/internal/textutil"
	"fmt"
	"strings"
)

// KindOf is the entry's kind, derived from its name when it does not declare one.
func (e Entry) KindOf() string {
	if e.Kind != "" {
		return e.Kind
	}
	switch storepath.Kind(e.Name) {
	case "scan":
		return KindScan
	case "page":
		return KindPage
	case "sidecar":
		return KindSidecar
	case "index":
		return KindIndex
	default:
		return KindScan
	}
}

// Dir is the staging directory of the entry: scans/<name> for a scan, sidecars/<name> for a
// sidecar. It is built from the validated name, never from a caller-supplied path.
func (e Entry) Dir() string {
	switch e.KindOf() {
	case KindSidecar, KindIndex:
		return "sidecars/" + e.Name
	default:
		return "scans/" + e.Name
	}
}

// PageFile is the name of the page image at index page (1-based) inside the entry directory.
func (e Entry) PageFile(page int) string { return fmt.Sprintf("page-%03d.pgm", page) }

// IndexFile is the name of the page index sidecar the scanner wrote for the entry.
const IndexFile = "page-index.tsv"

// ContainerFile is the name the scan container is stored under, derived from the entry name and its
// extension. A scan with no extension is stored as <name>.pdf.
func (e Entry) ContainerFile() string {
	if ext := storepath.Ext(e.Name); ext != "" {
		return e.Name
	}
	return e.Name + ".pdf"
}

// Describe renders the entry for a log line or an audit record.
func (e Entry) Describe() string {
	return textutil.Sentence(
		fmt.Sprintf("%s %s", e.KindOf(), e.Name),
		fmt.Sprintf("%d pages", e.Pages),
		fmt.Sprintf("%d bytes", e.Bytes),
		e.Title,
	)
}

// Label is the short label a page prints for the entry: the title when it has one, the name
// otherwise, shortened to the label limit.
func (e Entry) Label() string {
	if strings.TrimSpace(e.Title) != "" {
		return textutil.Label(e.Title)
	}
	return textutil.Label(e.Name)
}

// DigestShort is the digest prefix the report page shows next to an entry.
func (e Entry) DigestShort() string { return checksum.Short(e.Digest) }
