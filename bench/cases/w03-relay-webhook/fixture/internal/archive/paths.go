package archive

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Errors the path rule reports.
var (
	ErrOutside = errors.New("archive: path leaves the root")
	ErrBadName = errors.New("archive: invalid entry name")
)

// SafeJoin returns the path of an entry under root, and ErrOutside for a name that would leave it.
// Every write the pipeline performs goes through it, so an entry name is never trusted to be a
// relative name.
func SafeJoin(root, name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("%w: empty name", ErrBadName)
	}
	if strings.ContainsRune(name, 0) {
		return "", fmt.Errorf("%w: %q carries a NUL byte", ErrBadName, name)
	}
	if filepath.IsAbs(name) || strings.HasPrefix(name, "/") || strings.HasPrefix(name, `\`) {
		return "", fmt.Errorf("%w: %q is absolute", ErrBadName, name)
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(name)))
	// A tar entry may carry "./x" and "a//b"; clean first, then the rule, so the check is over the
	// path that would be written.
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("%w: %q walks upwards", ErrOutside, name)
	}
	joined := filepath.Join(root, filepath.FromSlash(clean))
	rel, err := filepath.Rel(root, joined)
	if err != nil {
		return "", fmt.Errorf("%w: %q", ErrOutside, name)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", fmt.Errorf("%w: %q", ErrOutside, name)
	}
	return joined, nil
}

// Write stores one entry under root through SafeJoin and returns the path it wrote. The directory
// of the entry is created first; the name is checked before anything is created, so a refused entry
// leaves no directory behind.
func Write(root string, entry Entry) (string, error) {
	target, err := SafeJoin(root, entry.Name)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", fmt.Errorf("archive: prepare %q: %w", entry.Name, err)
	}
	if err := os.WriteFile(target, entry.Body, 0o644); err != nil {
		return "", fmt.Errorf("archive: write %q: %w", entry.Name, err)
	}
	return target, nil
}

// Key renders a path relative to root with forward slashes, which is the form the report uses.
func Key(root, path string) (string, error) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", fmt.Errorf("archive: key of %q: %w", path, err)
	}
	return filepath.ToSlash(rel), nil
}
