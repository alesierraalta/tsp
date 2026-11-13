// Package storage is the artifact server's tree on disk: the blobs a job produced, the staging
// directories a pack reads, and the containment rule every path is resolved through.
package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Errors the tree reports.
var (
	// ErrInvalid is returned for a name the tree refuses.
	ErrInvalid = errors.New("storage: invalid name")
	// ErrOutside is returned for a path that leaves the root.
	ErrOutside = errors.New("storage: the path leaves the root")
	// ErrNotFound is returned for a file the tree does not carry.
	ErrNotFound = errors.New("storage: no such file")
)

// Join resolves a name under a root and proves the result is inside it.
//
// Every path this service reads or writes goes through Join: an absolute name, a name carrying a NUL
// byte, a name that walks upwards and a name that resolves through a symbolic link to somewhere else
// are all refused before anything is opened, so the root is the only place a name can point.
func Join(root, name string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("%w: the root is empty", ErrInvalid)
	}
	if strings.ContainsRune(name, 0) {
		return "", fmt.Errorf("%w: %q carries a NUL byte", ErrInvalid, name)
	}
	if name == "" {
		return "", fmt.Errorf("%w: the name is empty", ErrInvalid)
	}
	if filepath.IsAbs(name) {
		return "", fmt.Errorf("%w: %q is absolute", ErrInvalid, name)
	}
	cleanRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	joined := filepath.Join(cleanRoot, name)
	relative, err := filepath.Rel(cleanRoot, joined)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: %q walks upwards", ErrOutside, name)
	}
	if relative == "." {
		return cleanRoot, nil
	}
	return joined, nil
}

// EnsureDir creates a directory under the root, refusing a name that leaves it.
func EnsureDir(root, name string, mode os.FileMode) (string, error) {
	path, err := Join(root, name)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(path, mode); err != nil {
		return "", fmt.Errorf("storage: %w", err)
	}
	return path, nil
}

// OpenFile returns the file a name points at, refusing a path that does not sit under the root once
// the symbolic links are resolved.
func OpenFile(root, name string) (*os.File, error) {
	path, err := Join(root, name)
	if err != nil {
		return nil, err
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %q", ErrNotFound, name)
		}
		return nil, fmt.Errorf("storage: %w", err)
	}
	cleanRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("storage: %w", err)
	}
	if !strings.HasPrefix(resolved, cleanRoot+string(filepath.Separator)) && resolved != cleanRoot {
		return nil, fmt.Errorf("%w: %q resolves outside the root", ErrOutside, name)
	}
	return os.Open(resolved)
}

// Write stores a body under a name, creating the file and refusing a name that leaves the root.
func Write(root, name string, body []byte, mode os.FileMode) (string, error) {
	path, err := Join(root, name)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("storage: %w", err)
	}
	if err := os.WriteFile(path, body, mode); err != nil {
		return "", fmt.Errorf("storage: %w", err)
	}
	return path, nil
}

// Read returns the body of a file under the root.
func Read(root, name string) ([]byte, error) {
	file, err := OpenFile(root, name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("storage: %w", err)
	}
	body := make([]byte, info.Size())
	if _, err := file.Read(body); err != nil {
		return nil, fmt.Errorf("storage: %w", err)
	}
	return body, nil
}

// Remove deletes a file under the root.
func Remove(root, name string) error {
	path, err := Join(root, name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %q", ErrNotFound, name)
		}
		return fmt.Errorf("storage: %w", err)
	}
	return nil
}

// Exists reports whether a file under the root exists.
func Exists(root, name string) bool {
	path, err := Join(root, name)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// Size returns the size of a file under the root.
func Size(root, name string) (int64, error) {
	path, err := Join(root, name)
	if err != nil {
		return 0, err
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, fmt.Errorf("%w: %q", ErrNotFound, name)
		}
		return 0, fmt.Errorf("storage: %w", err)
	}
	return info.Size(), nil
}
