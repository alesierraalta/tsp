package storepath

import (
	"fmt"
	"path"
	"strings"
)

// maxSegment is the longest single path segment the service accepts. It is longer than any name a
// scan produces and short enough that a name cannot be used to build a page-long key.
const maxSegment = 96

// extensions is the set of object extensions the archive knows. A rendition is a PGM page image, a
// sidecar is the OCR text of one page, and the rest are the containers a scan arrives in.
var extensions = map[string]string{
	".pdf":  "scan",
	".tif":  "scan",
	".tiff": "scan",
	".pgm":  "page",
	".txt":  "sidecar",
	".tsv":  "sidecar",
	".json": "manifest",
	".csv":  "index",
}

// Segment refuses anything that is not exactly one path segment with an extension the archive
// knows. An attachment, a rendition and a sidecar are flat by contract; a nested or unknown name is
// refused before it reaches a directory listing.
func Segment(name string) error {
	if err := CheckName(name); err != nil {
		return err
	}
	trimmed := strings.TrimSpace(name)
	if strings.ContainsAny(trimmed, `/\`) {
		return fmt.Errorf("%w: %q is not one segment", ErrInvalid, name)
	}
	if len(trimmed) > maxSegment {
		return fmt.Errorf("%w: %q is longer than %d bytes", ErrInvalid, name, maxSegment)
	}
	if _, ok := extensions[strings.ToLower(path.Ext(trimmed))]; !ok {
		return fmt.Errorf("%w: %q has no known extension", ErrInvalid, name)
	}
	return nil
}

// Ext returns the lowercased extension of a name, or "" when it has none.
func Ext(name string) string { return strings.ToLower(path.Ext(strings.TrimSpace(name))) }

// Kind names the role a known extension plays: scan, page, sidecar, manifest or index. An unknown
// extension is "other" rather than an error, so a listing can report what it found.
func Kind(name string) string {
	if kind, ok := extensions[Ext(name)]; ok {
		return kind
	}
	return "other"
}

// Base returns the name without its directory part, for the places a listing shows a label. It does
// not resolve anything and never decides where a file is read from.
func Base(name string) string {
	return path.Base(filepathSlash(name))
}

// DirOf returns the directory part of a slash-separated key, or "." for a top-level key.
func DirOf(key string) string {
	dir := path.Dir(filepathSlash(key))
	if dir == "" {
		return "."
	}
	return dir
}

// Stem returns the name without its extension, which is how a page index refers to its scan.
func Stem(name string) string {
	base := Base(name)
	ext := path.Ext(base)
	if ext == "" {
		return base
	}
	return strings.TrimSuffix(base, ext)
}

// filepathSlash swaps the platform separator for a slash without touching the path package, so a
// caller on Windows and a caller on Linux read the same key.
func filepathSlash(name string) string {
	return strings.ReplaceAll(name, `\`, "/")
}
