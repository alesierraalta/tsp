// Package vault owns the document tree of one docvault instance: the objects on disk and the
// metadata the service keeps for them.
package vault

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Errors the callers translate into HTTP status codes.
var (
	ErrOutside  = errors.New("vault: path leaves the root")
	ErrInvalid  = errors.New("vault: invalid name")
	ErrNoObject = errors.New("vault: no such object")
	ErrConflict = errors.New("vault: object already exists")
)

// Vault is a document tree under one absolute root plus the metadata kept for its objects.
type Vault struct {
	root  string
	store *Store
}

// Open prepares a vault at root. The root is made absolute and cleaned before use; a root that is
// the filesystem root is refused, because every containment check would then be vacuous.
func Open(root string) (*Vault, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("vault: root %q: %w", root, err)
	}
	abs = filepath.Clean(abs)
	if abs == string(filepath.Separator) {
		return nil, fmt.Errorf("%w: the filesystem root is not a vault", ErrInvalid)
	}
	return &Vault{root: abs, store: NewStore()}, nil
}

// Root is the absolute root of the tree. It is a value, not a handle: callers that need a path
// inside the tree ask Resolve, never Root plus their own join.
func (v *Vault) Root() string { return v.root }

// Store is the metadata store backing the vault. The metadata it carries is what the API renders;
// the object bytes are read through the methods below.
func (v *Vault) Store() *Store { return v.store }

// Resolve returns the absolute path of an object name, and ErrOutside for a name that would leave
// the root. Every filesystem read and write in the service goes through this method or through the
// helpers built on it.
func (v *Vault) Resolve(name string) (string, error) {
	return resolve(v.root, name)
}

// Key returns the object key of a name: the path relative to the root with forward slashes.
func (v *Vault) Key(name string) (string, error) {
	resolved, err := v.Resolve(name)
	if err != nil {
		return "", err
	}
	return key(v.root, resolved)
}

// Read returns the bytes of an object, and ErrOutside for a name that would leave the root.
func (v *Vault) Read(name string) ([]byte, error) {
	resolved, err := v.Resolve(name)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(resolved)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: %q", ErrNoObject, name)
	}
	if err != nil {
		return nil, fmt.Errorf("vault: read %q: %w", name, err)
	}
	return data, nil
}

// Write stores an object, creating its directory. The name is resolved first, so a write can never
// land outside the root.
func (v *Vault) Write(name string, data []byte) error {
	resolved, err := v.Resolve(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return fmt.Errorf("vault: prepare %q: %w", name, err)
	}
	if err := os.WriteFile(resolved, data, 0o644); err != nil {
		return fmt.Errorf("vault: write %q: %w", name, err)
	}
	return nil
}

// Stat reports the size and modification time of an object.
func (v *Vault) Stat(name string) (int64, time.Time, error) {
	resolved, err := v.Resolve(name)
	if err != nil {
		return 0, time.Time{}, err
	}
	info, err := os.Stat(resolved)
	if errors.Is(err, os.ErrNotExist) {
		return 0, time.Time{}, fmt.Errorf("%w: %q", ErrNoObject, name)
	}
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("vault: stat %q: %w", name, err)
	}
	return info.Size(), info.ModTime(), nil
}

// List returns the object keys under a prefix inside the tree, sorted. The prefix is resolved like
// any other name, so listing outside the root is refused rather than clamped.
func (v *Vault) List(prefix string) ([]string, error) {
	base, err := v.Resolve(prefix)
	if err != nil {
		return nil, err
	}
	var keys []string
	err = filepath.Walk(base, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil
			}
			return err
		}
		if info.IsDir() {
			return nil
		}
		k, err := key(v.root, p)
		if err != nil {
			return err
		}
		keys = append(keys, k)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("vault: list %q: %w", prefix, err)
	}
	return keys, nil
}

// Remove deletes an object and its metadata.
func (v *Vault) Remove(name string) error {
	resolved, err := v.Resolve(name)
	if err != nil {
		return err
	}
	k, err := key(v.root, resolved)
	if err != nil {
		return err
	}
	if err := os.Remove(resolved); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("vault: remove %q: %w", name, err)
	}
	v.store.Delete(k)
	return nil
}

// Normalize turns a client-supplied name into the form the tree stores: forward slashes, no
// leading "./", no trailing separator. It refuses nothing; Resolve is the call that decides
// whether the name may be used.
func (v *Vault) Normalize(name string) string {
	n := filepath.ToSlash(strings.TrimSpace(name))
	n = strings.TrimPrefix(n, "./")
	n = strings.TrimSuffix(n, "/")
	return n
}
