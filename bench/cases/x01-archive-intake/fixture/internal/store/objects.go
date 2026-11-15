package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"example.com/archive/internal/checksum"
	"example.com/archive/internal/storepath"
)

// blobName is the name a digest is stored under: the first two hex characters as a directory, the
// full digest as the file. The split keeps one directory from carrying every blob.
func blobName(digest string) string {
	if len(digest) < 4 {
		return digest
	}
	return digest[:2] + "/" + digest
}

// blobPath resolves the path of a digest through the containment rule. A digest is validated first,
// so a value that is not the hex of the archive's algorithm cannot steer the join.
func (s *Store) blobPath(digest string) (string, error) {
	if err := checksum.Validate(digest); err != nil {
		return "", fmt.Errorf("store: %w", err)
	}
	return s.path(filepath.ToSlash(filepath.Join("blobs", blobName(digest))))
}

// writeBlob stores content under its digest. An existing blob is left alone: two entries with the
// same content share one file, and rewriting it would race a reader for no gain.
func (s *Store) writeBlob(digest string, data []byte) error {
	target, err := s.blobPath(digest)
	if err != nil {
		return err
	}
	if _, err := os.Stat(target); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("store: create blob dir: %w", err)
	}
	if err := os.WriteFile(target, data, 0o644); err != nil {
		return fmt.Errorf("store: write blob: %w", err)
	}
	return nil
}

// readBlob reads content by digest and checks it: a blob that no longer matches its name is reported
// rather than served.
func (s *Store) readBlob(digest string) ([]byte, error) {
	target, err := s.blobPath(digest)
	if err != nil {
		return nil, err
	}
	body, err := os.ReadFile(target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: blob %s", ErrNotFound, checksum.Short(digest))
		}
		return nil, fmt.Errorf("store: read blob: %w", err)
	}
	if checksum.Sum(body) != digest {
		return nil, fmt.Errorf("store: blob %s does not match its digest", checksum.Short(digest))
	}
	return body, nil
}

// metaPath resolves the metadata file of an object. The id is validated before it becomes a file
// name, so a caller cannot point the store at another file.
func (s *Store) metaPath(id string) (string, error) {
	if !validObjectID(id) {
		return "", fmt.Errorf("%w: id %q", ErrInvalid, id)
	}
	return s.path("meta/" + id + ".json")
}

// textPath resolves the extracted text file of an object.
func (s *Store) textPath(id string) (string, error) {
	if !validObjectID(id) {
		return "", fmt.Errorf("%w: id %q", ErrInvalid, id)
	}
	return s.path("text/" + id + ".txt")
}

// writeMeta stores a record as JSON.
func (s *Store) writeMeta(record Record) error {
	raw, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("store: encode meta: %w", err)
	}
	target, err := s.metaPath(record.ID)
	if err != nil {
		return err
	}
	if err := os.WriteFile(target, append(raw, '\n'), 0o644); err != nil {
		return fmt.Errorf("store: write meta: %w", err)
	}
	return nil
}

// readMeta reads one record from its file.
func (s *Store) readMeta(id string) (Record, error) {
	target, err := s.metaPath(id)
	if err != nil {
		return Record{}, err
	}
	body, err := os.ReadFile(target)
	if err != nil {
		if os.IsNotExist(err) {
			return Record{}, fmt.Errorf("%w: %s", ErrNotFound, id)
		}
		return Record{}, fmt.Errorf("store: read meta: %w", err)
	}
	var record Record
	if err := json.Unmarshal(body, &record); err != nil {
		return Record{}, fmt.Errorf("store: decode meta %s: %w", id, err)
	}
	if record.ID != id {
		return Record{}, fmt.Errorf("store: meta %s names %s", id, record.ID)
	}
	return record, nil
}

// writeText stores the extracted text of an object.
func (s *Store) writeText(id string, text []byte) error {
	target, err := s.textPath(id)
	if err != nil {
		return err
	}
	if err := os.WriteFile(target, text, 0o644); err != nil {
		return fmt.Errorf("store: write text: %w", err)
	}
	return nil
}

// readTextFile reads the extracted text of an object.
func (s *Store) readTextFile(id string) ([]byte, error) {
	target, err := s.textPath(id)
	if err != nil {
		return nil, err
	}
	body, err := os.ReadFile(target)
	if err != nil {
		return nil, err
	}
	return body, nil
}

// BlobPath is the path a blob is stored at, for the operator page. It is read-only and validated like
// every other path.
func (s *Store) BlobPath(digest string) (string, error) { return s.blobPath(digest) }

// validObjectID is the id shape check, kept local so every path helper uses the same rule.
func validObjectID(id string) bool {
	if id == "" {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
		default:
			return false
		}
	}
	return len(id) <= 64
}

// KeyOf renders a stored blob's key relative to the objects directory.
func (s *Store) KeyOf(digest string) string {
	return filepath.ToSlash(filepath.Join("blobs", blobName(digest)))
}

// ResolveMeta is the path helper the report uses to show where an object lives, expressed as a key.
func (s *Store) ResolveMeta(id string) (string, error) {
	target, err := s.metaPath(id)
	if err != nil {
		return "", err
	}
	return storepath.Key(s.base, target)
}
