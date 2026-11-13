package packaging

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"example.com/artifacts/internal/execx"
)

// Manifest is the description a pack leaves beside its archive: what was packed, what the archive
// weighs and what it carries.
type Manifest struct {
	Name     string    `json:"name"`
	Kind     string    `json:"kind"`
	Archive  string    `json:"archive"`
	Bytes    int64     `json:"bytes"`
	Digest   string    `json:"digest"`
	Entries  []string  `json:"entries"`
	Tool     string    `json:"tool,omitempty"`
	PackedAt time.Time `json:"packed_at"`
}

// ManifestName is the file name a manifest is written under.
const ManifestName = "pack.json"

// NewManifest describes a pack that finished.
func NewManifest(result Result, digest string, at time.Time) (Manifest, error) {
	if err := result.Validate(); err != nil {
		return Manifest{}, err
	}
	if strings.TrimSpace(digest) == "" {
		return Manifest{}, fmt.Errorf("packaging: the manifest needs a digest")
	}
	return Manifest{
		Name: result.Name, Kind: result.Kind, Archive: filepath.Base(result.Path),
		Bytes: result.Bytes, Digest: digest, Entries: append([]string(nil), result.Entries...),
		Tool: result.Tool, PackedAt: at.UTC(),
	}, nil
}

// Validate checks the manifest.
func (m Manifest) Validate() error {
	if err := execx.CheckName(m.Name); err != nil {
		return fmt.Errorf("%w: %v", ErrName, err)
	}
	if !knownKind(m.Kind) {
		return fmt.Errorf("%w: %q", ErrKind, m.Kind)
	}
	if m.Bytes < 0 {
		return fmt.Errorf("%w: the manifest reports %d bytes", ErrRequest, m.Bytes)
	}
	if len(m.Digest) != 64 {
		return fmt.Errorf("%w: the digest is %q", ErrRequest, m.Digest)
	}
	if len(m.Entries) == 0 {
		return fmt.Errorf("%w: the manifest carries no entries", ErrRequest)
	}
	for _, entry := range m.Entries {
		if err := execx.CheckArg(entry); err != nil {
			return fmt.Errorf("%w: %v", ErrName, err)
		}
	}
	return nil
}

// Write stores the manifest beside the archive it describes.
func (m Manifest) Write(dir string) (string, error) {
	if err := m.Validate(); err != nil {
		return "", err
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("%w: %q is not a directory", ErrRequest, dir)
	}
	body, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", fmt.Errorf("packaging: %w", err)
	}
	path := filepath.Join(dir, ManifestName)
	if err := os.WriteFile(path, append(body, '\n'), 0o644); err != nil {
		return "", fmt.Errorf("packaging: %w", err)
	}
	return path, nil
}

// ReadManifest reads the manifest of a pack directory.
func ReadManifest(dir string) (Manifest, error) {
	body, err := os.ReadFile(filepath.Join(dir, ManifestName))
	if err != nil {
		return Manifest{}, fmt.Errorf("packaging: %w", err)
	}
	var manifest Manifest
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("packaging: %w", err)
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

// EntriesMatch reports whether what an archive carries is what the pack reported.
func EntriesMatch(reported, found []string) bool {
	if len(reported) != len(found) {
		return false
	}
	for i, name := range reported {
		if name != found[i] {
			return false
		}
	}
	return true
}
