package manifest

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"example.com/archive/internal/clock"
)

// Parse reads one manifest from r. The reader is bounded by the limits' body cap, unknown fields are
// refused rather than ignored, and a second JSON value after the manifest is an error: a sender that
// appends a second document is not describing one batch.
func Parse(r io.Reader, limits Limits) (Manifest, error) {
	lim := limits.Or()
	if lim.MaxBodyBytes <= 0 {
		return Manifest{}, fmt.Errorf("manifest: no body cap")
	}
	body, err := readBounded(r, lim.MaxBodyBytes)
	if err != nil {
		return Manifest{}, err
	}
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.DisallowUnknownFields()
	var m Manifest
	if err := dec.Decode(&m); err != nil {
		return Manifest{}, fmt.Errorf("manifest: decode: %w", err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return Manifest{}, fmt.Errorf("manifest: a second JSON value follows the manifest")
		}
		return Manifest{}, fmt.Errorf("manifest: trailing data: %w", err)
	}
	if err := m.Validate(lim); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

// readBounded reads at most limit bytes and refuses a stream that is longer, instead of decoding a
// prefix of a larger document.
func readBounded(r io.Reader, limit int64) ([]byte, error) {
	if r == nil {
		return nil, errors.New("manifest: no reader")
	}
	buf := make([]byte, 0, 4096)
	chunk := make([]byte, 4096)
	for {
		n, err := r.Read(chunk)
		if n > 0 {
			if int64(len(buf))+int64(n) > limit {
				return nil, fmt.Errorf("manifest: body is longer than %d bytes", limit)
			}
			buf = append(buf, chunk[:n]...)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("manifest: read: %w", err)
		}
	}
	if len(buf) == 0 {
		return nil, errors.New("manifest: empty body")
	}
	return buf, nil
}

// Decode is Parse for a caller that already holds the bytes, such as the pipeline reading a manifest
// back from its staging directory.
func Decode(body []byte, limits Limits) (Manifest, error) {
	return Parse(strings.NewReader(string(body)), limits)
}

// Encode renders a manifest the way the staging tree stores it: indented, one entry per block, with
// a trailing newline.
func Encode(m Manifest) ([]byte, error) {
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("manifest: encode: %w", err)
	}
	return append(raw, '\n'), nil
}

// Validate checks the fields a stage reads. It runs once, at accept time, so no stage has to repeat
// the checks and no stage can be reached with a value the service would refuse.
func (m *Manifest) Validate(limits Limits) error {
	lim := limits.Or()
	if err := lim.Check(); err != nil {
		return err
	}
	if strings.TrimSpace(m.Tenant) == "" {
		return fmt.Errorf("manifest: tenant is empty")
	}
	created, err := normalizeCreated(m.Created)
	if err != nil {
		return err
	}
	m.Created = created
	if len(m.Entries) == 0 {
		return fmt.Errorf("manifest: no entries")
	}
	if len(m.Entries) > lim.MaxEntries {
		return fmt.Errorf("manifest: %d entries, more than the %d the deployment accepts", len(m.Entries), lim.MaxEntries)
	}
	seen := map[string]bool{}
	for i := range m.Entries {
		if err := m.Entries[i].validate(lim); err != nil {
			return fmt.Errorf("manifest: entry %d: %w", i, err)
		}
		name := m.Entries[i].Name
		if seen[name] {
			return fmt.Errorf("manifest: entry name %q repeated", name)
		}
		seen[name] = true
	}
	if total := m.TotalBytes(); total > lim.MaxUpload {
		return fmt.Errorf("manifest: declares %d bytes, more than the %d the deployment accepts", total, lim.MaxUpload)
	}
	if pages := m.Pages(); pages > lim.MaxPages {
		return fmt.Errorf("manifest: declares %d pages, more than the %d the deployment accepts", pages, lim.MaxPages)
	}
	return nil
}

// normalizeCreated reads the creation instant and renders it back in the canonical form the service
// stores, so a manifest re-read from staging validates byte for byte.
func normalizeCreated(s string) (string, error) {
	t, err := clock.ParseRFC3339(s)
	if err != nil {
		return "", fmt.Errorf("manifest: created: %w", err)
	}
	return clock.Stamp(t), nil
}
