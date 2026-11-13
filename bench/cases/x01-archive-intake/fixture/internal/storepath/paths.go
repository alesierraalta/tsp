// Package storepath is the one place where a name becomes a filesystem path. The intake service
// resolves every path it reads, writes or lists through this package, so the containment rule is
// stated once and no caller has to repeat it.
package storepath

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"
)

// Errors a refused name carries. Callers map them to a status code; nothing is read or written for
// a name that produces one.
var (
	ErrInvalid = errors.New("storepath: invalid name")
	ErrOutside = errors.New("storepath: outside the root")
)

// sep is the platform separator the containment comparison uses. It is derived once so the check is
// a string comparison rather than a second filesystem call.
const sep = string(filepath.Separator)

// CheckName rejects the names no object may carry before any join happens: an empty name, a name
// with a NUL byte, an absolute name, and a name whose cleaned form still walks upwards.
func CheckName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: empty name", ErrInvalid)
	}
	if strings.ContainsRune(name, 0) {
		return fmt.Errorf("%w: name carries a NUL byte", ErrInvalid)
	}
	if strings.HasPrefix(name, "/") || strings.HasPrefix(name, `\`) || filepath.IsAbs(name) {
		return fmt.Errorf("%w: absolute name %q", ErrInvalid, name)
	}
	clean := path.Clean(filepath.ToSlash(name))
	if clean == "." {
		return fmt.Errorf("%w: name %q is the root itself", ErrInvalid, name)
	}
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return fmt.Errorf("%w: name %q walks upwards", ErrInvalid, name)
	}
	return nil
}

// Resolve joins name onto root and proves that the result stays inside root. The check is done on
// the joined path as well as on the name, so a name that looks harmless but lands outside after
// cleaning is refused too. Every path this service hands to the filesystem comes from here.
func Resolve(root, name string) (string, error) {
	if err := CheckName(name); err != nil {
		return "", err
	}
	joined := filepath.Join(root, filepath.FromSlash(name))
	if !Contained(root, joined) {
		return "", fmt.Errorf("%w: %q", ErrOutside, name)
	}
	return joined, nil
}

// Contained reports whether p is root or sits under it. The separator after the prefix is what
// keeps a sibling directory that merely shares the root's name out of the tree: "/srv/archive-copy"
// is not inside "/srv/archive".
func Contained(root, p string) bool {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(p))
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

// Key renders a resolved path as the object key the store and the audit trail use: relative to the
// root, with forward slashes, so a key is the same string on every platform.
func Key(root, resolved string) (string, error) {
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(resolved))
	if err != nil {
		return "", fmt.Errorf("storepath: key of %q: %w", resolved, err)
	}
	if rel == "." {
		return "", fmt.Errorf("%w: %q is the root itself", ErrInvalid, resolved)
	}
	return filepath.ToSlash(rel), nil
}

// Under reports whether key, read as a slash-separated relative name, sits in dir. It is the check
// the listing filters use, and it reads names, never paths, so it cannot be talked into leaving the
// root: "a/../b" is cleaned before the comparison.
func Under(dir, key string) bool {
	cleanDir := path.Clean(filepath.ToSlash(dir))
	cleanKey := path.Clean(filepath.ToSlash(key))
	if cleanDir == "." || cleanDir == "/" {
		return cleanKey != ".." && !strings.HasPrefix(cleanKey, "../")
	}
	return cleanKey == cleanDir || strings.HasPrefix(cleanKey, cleanDir+"/")
}

// Slash renders a name in the canonical form the service stores and reports: forward slashes, no
// leading "./", no trailing slash.
func Slash(name string) string {
	trimmed := filepath.ToSlash(strings.TrimSpace(name))
	trimmed = strings.TrimPrefix(trimmed, "./")
	return strings.TrimSuffix(trimmed, "/")
}
