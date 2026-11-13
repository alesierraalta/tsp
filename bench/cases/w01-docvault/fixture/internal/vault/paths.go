package vault

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

// sep is the separator the containment check compares with. Derived from the platform so the
// comparison is a string test rather than a second filesystem call.
const sep = string(filepath.Separator)

// checkName rejects the names no object may carry before any join happens: an empty name, a name
// with a NUL byte, an absolute name, and a name whose cleaned form still walks upwards.
func checkName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: empty name", ErrInvalid)
	}
	if strings.ContainsRune(name, 0) {
		return fmt.Errorf("%w: name carries a NUL byte", ErrInvalid)
	}
	if strings.HasPrefix(name, "/") || filepath.IsAbs(name) {
		return fmt.Errorf("%w: absolute name %q", ErrInvalid, name)
	}
	clean := path.Clean(filepath.ToSlash(name))
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("%w: name %q walks upwards", ErrInvalid, name)
	}
	return nil
}

// resolve joins name onto root and proves the result stays inside root. The check is done on the
// joined path as well as on the name, so a name that is harmless on its own but lands outside after
// cleaning is refused too. Every filesystem path this package hands out comes from here.
func resolve(root, name string) (string, error) {
	if err := checkName(name); err != nil {
		return "", err
	}
	joined := filepath.Join(root, filepath.FromSlash(name))
	if !contained(root, joined) {
		return "", fmt.Errorf("%w: %q", ErrOutside, name)
	}
	return joined, nil
}

// contained reports whether p is root or sits under it. The separator after the prefix is what
// keeps a sibling directory that merely shares the root's name out of the tree: "/srv/vault-copy"
// is not inside "/srv/vault".
func contained(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+sep) {
		return false
	}
	return !filepath.IsAbs(rel)
}

// key renders a resolved path as the object key the store and the audit log use: relative to the
// root with forward slashes.
func key(root, resolved string) (string, error) {
	rel, err := filepath.Rel(root, resolved)
	if err != nil {
		return "", fmt.Errorf("vault: key of %q: %w", resolved, err)
	}
	return filepath.ToSlash(rel), nil
}
