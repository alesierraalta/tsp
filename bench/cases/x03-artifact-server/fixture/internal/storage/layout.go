package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Layout is where the server keeps each kind of file under its root.
type Layout struct {
	root string
}

// Open validates a root and returns its layout. The root must be absolute, must not be the filesystem
// root and must be readable and writable, because every write the server performs lands inside it.
func Open(root string) (*Layout, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("%w: the root is empty", ErrInvalid)
	}
	clean, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if clean == string(filepath.Separator) {
		return nil, fmt.Errorf("%w: the root may not be the filesystem root", ErrInvalid)
	}
	if err := os.MkdirAll(clean, 0o755); err != nil {
		return nil, fmt.Errorf("storage: %w", err)
	}
	return &Layout{root: clean}, nil
}

// Root returns the absolute root.
func (l *Layout) Root() string { return l.root }

// Dir returns one of the tree's directories, creating it.
func (l *Layout) Dir(name string) (string, error) {
	switch name {
	case "blobs", "staging", "packs", "manifests":
	default:
		return "", fmt.Errorf("%w: %q is not a directory of the tree", ErrInvalid, name)
	}
	return EnsureDir(l.root, name, 0o755)
}

// StagingDir returns the staging directory of one job, creating it.
func (l *Layout) StagingDir(job string) (string, error) {
	if err := validSegment(job); err != nil {
		return "", err
	}
	return EnsureDir(l.root, filepath.Join("staging", job), 0o755)
}

// PackDir returns the directory a job's archives are written to, creating it.
func (l *Layout) PackDir(job string) (string, error) {
	if err := validSegment(job); err != nil {
		return "", err
	}
	return EnsureDir(l.root, filepath.Join("packs", job), 0o755)
}

// ManifestDir returns the directory a job's manifests are written to, creating it.
func (l *Layout) ManifestDir(job string) (string, error) {
	if err := validSegment(job); err != nil {
		return "", err
	}
	return EnsureDir(l.root, filepath.Join("manifests", job), 0o755)
}

// Drop removes a job's staging tree once its pack finished, so an interrupted job leaves nothing a
// later job could read.
func (l *Layout) Drop(job string) error {
	if err := validSegment(job); err != nil {
		return err
	}
	path, err := Join(l.root, filepath.Join("staging", job))
	if err != nil {
		return err
	}
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("storage: %w", err)
	}
	return nil
}

// Contains reports whether a path is inside the root, following no symbolic link.
func (l *Layout) Contains(path string) bool {
	clean, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	return clean == l.root || strings.HasPrefix(clean, l.root+string(filepath.Separator))
}

// validSegment reports whether a value can be one segment of a path.
func validSegment(value string) error {
	if value == "" || value == "." || value == ".." {
		return fmt.Errorf("%w: %q is not a path segment", ErrInvalid, value)
	}
	if strings.ContainsAny(value, "/\\\x00") {
		return fmt.Errorf("%w: %q carries a separator", ErrInvalid, value)
	}
	return nil
}
