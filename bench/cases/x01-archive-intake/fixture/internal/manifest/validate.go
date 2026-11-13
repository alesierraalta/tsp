package manifest

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"example.com/archive/internal/checksum"
	"example.com/archive/internal/storepath"
	"example.com/archive/internal/textutil"
)

// Caps the entry rules apply. They are smaller than the deployment's caps on purpose: an entry larger
// than this is not a paper record, and a sender that declares one is describing something else.
const (
	MaxNameBytes  = 64
	MaxTitleBytes = 200
	MaxNoteBytes  = 2000
	MaxTagBytes   = 32
	MaxTags       = 8
	MaxSourceLen  = 256
)

// validate checks one entry. The name is a flat, validated segment; every other field is bounded and
// printable; a declared digest is well formed before anything compares it.
func (e *Entry) validate(lim Limits) error {
	if err := validateName(e.Name); err != nil {
		return err
	}
	if e.Kind != "" {
		switch e.Kind {
		case KindScan, KindPage, KindSidecar, KindIndex:
		default:
			return fmt.Errorf("kind %q is not scan, page, sidecar or index", e.Kind)
		}
	}
	if e.Kind == "" {
		e.Kind = e.KindOf()
	}
	if e.Kind == KindScan && e.Pages <= 0 {
		return fmt.Errorf("scan %q declares no pages", e.Name)
	}
	if e.Kind != KindScan && e.Pages != 0 {
		return fmt.Errorf("%s %q declares pages, which only a scan carries", e.Kind, e.Name)
	}
	if lim.MaxPages > 0 && e.Pages > lim.MaxPages {
		return fmt.Errorf("%q declares %d pages, more than the %d one entry may declare", e.Name, e.Pages, lim.MaxPages)
	}
	if e.Bytes < 0 {
		return fmt.Errorf("%q declares a negative size", e.Name)
	}
	if lim.MaxUpload > 0 && e.Bytes > lim.MaxUpload {
		return fmt.Errorf("%q declares %d bytes, more than the %d one object may carry", e.Name, e.Bytes, lim.MaxUpload)
	}
	if e.Digest != "" {
		if err := checksum.Validate(e.Digest); err != nil {
			return fmt.Errorf("%q: %w", e.Name, err)
		}
	}
	if err := boundedText("title", e.Title, MaxTitleBytes); err != nil {
		return fmt.Errorf("%q: %w", e.Name, err)
	}
	if err := boundedText("note", e.Note, MaxNoteBytes); err != nil {
		return fmt.Errorf("%q: %w", e.Name, err)
	}
	if len(e.Tags) > MaxTags {
		return fmt.Errorf("%q carries %d tags, more than %d", e.Name, len(e.Tags), MaxTags)
	}
	for i, tag := range e.Tags {
		if err := boundedText(fmt.Sprintf("tag %d", i), tag, MaxTagBytes); err != nil {
			return fmt.Errorf("%q: %w", e.Name, err)
		}
		if strings.TrimSpace(tag) != tag {
			return fmt.Errorf("%q: tag %q is not trimmed", e.Name, tag)
		}
	}
	if e.Source != "" {
		if err := ValidateSource(e.Source); err != nil {
			return fmt.Errorf("%q: %w", e.Name, err)
		}
	}
	e.Tags = textutil.SplitTags(strings.Join(e.Tags, ","))
	return nil
}

// validateName refuses a name that is not exactly one flat segment of the archive's alphabet. The
// name becomes a directory in the staging tree, so a separator, a dot segment or a control character
// is refused here rather than discovered by a caller that joins it.
func validateName(name string) error {
	if name == "" {
		return fmt.Errorf("name is empty")
	}
	if len(name) > MaxNameBytes {
		return fmt.Errorf("name %q is longer than %d bytes", name, MaxNameBytes)
	}
	if strings.TrimSpace(name) != name {
		return fmt.Errorf("name %q carries leading or trailing space", name)
	}
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("name %q carries a path separator", name)
	}
	if name == "." || name == ".." {
		return fmt.Errorf("name %q is a dot segment", name)
	}
	if err := storepath.CheckName(name); err != nil {
		return fmt.Errorf("name: %w", err)
	}
	if !textutil.Printable(name) {
		return fmt.Errorf("name %q carries a control character", name)
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-' || r == '_' || r == '.':
		default:
			return fmt.Errorf("name %q carries %q, which is not in a-z, 0-9, dash, underscore or dot", name, r)
		}
	}
	return nil
}

// boundedText refuses text that is longer than maxBytes or carries a control character. The check
// counts bytes because the limits do, and a title is stored as bytes.
func boundedText(label, value string, maxBytes int) error {
	if len(value) > maxBytes {
		return fmt.Errorf("%s is longer than %d bytes", label, maxBytes)
	}
	if !utf8.ValidString(value) {
		return fmt.Errorf("%s is not valid UTF-8", label)
	}
	if !textutil.Printable(value) {
		return fmt.Errorf("%s carries a control character", label)
	}
	for _, r := range value {
		if r == utf8.RuneError {
			return fmt.Errorf("%s carries an undecodable rune", label)
		}
		if unicode.IsSpace(r) && r != ' ' && r != '\n' && r != '\t' {
			return fmt.Errorf("%s carries a non-printing space", label)
		}
	}
	return nil
}

// ValidateManifest is the free-function form of Validate, for a caller that holds a value rather than
// a pointer.
func ValidateManifest(m Manifest, limits Limits) (Manifest, error) {
	copy := m
	if err := copy.Validate(limits); err != nil {
		return Manifest{}, err
	}
	return copy, nil
}
