// Package workspace is the directory tree the service keeps its statements, labels, exports and
// event payloads in, and the only code that turns a name into a filesystem path. A name is a
// slash-separated relative name such as "partners/acme/statements/2024-05/statement.html". It is
// resolved here once, against the configured root, and the resolved path is what the callers use: no
// other package joins a path of its own, which is what keeps a name from deciding where the service
// reads and writes.
package workspace

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

// Errors the path rule reports. Callers compare with errors.Is.
var (
	ErrOutside    = errors.New("workspace: path leaves the root")
	ErrBadName    = errors.New("workspace: invalid name")
	ErrNoObject   = errors.New("workspace: no such object")
	ErrNotRegular = errors.New("workspace: object is not a regular file")
	ErrTooLarge   = errors.New("workspace: object is too large")
)

// sep is the separator the containment comparison is built from. Deriving it from the platform keeps
// the rule and filepath.Join in agreement about what separates two names.
const sep = string(filepath.Separator)

// segmentRE is the form of the variable parts of a name: a tenant, partner, carrier or shipment id.
// The workspace checks them itself instead of trusting the caller, because they are joined onto the
// root. The pattern is anchored and the length is bounded by the repetition counts, so a value cannot
// borrow a byte from its neighbour and pass.
var segmentRE = regexp.MustCompile(`^[a-z][a-z0-9-]{0,30}[a-z0-9]$`)

// identifierRE is the form of a minted identifier such as "shp_9f2c...": a lowercase prefix, an
// underscore and lowercase hex. It is wider than a segment because an identifier carries the
// underscore the generator writes, and narrower in that it has no dashes.
var identifierRE = regexp.MustCompile(`^[a-z]{2,12}_[0-9a-f]{32}$`)

// ValidSegment reports whether a value may be one part of a name.
func ValidSegment(value string) bool { return segmentRE.MatchString(value) }

// ValidIdentifier reports whether a value may be one part of a name as an identifier. A tenant, a
// partner and a report are segments; a shipment and an event are minted identifiers.
func ValidIdentifier(value string) bool { return identifierRE.MatchString(value) }

// Workspace is one directory under its absolute root.
type Workspace struct {
	root string
}

// Open prepares the workspace at root. The root is made absolute and cleaned first; a root that is
// the filesystem root is refused, because every containment check would then be vacuous.
func Open(root string) (*Workspace, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("%w: empty root", ErrBadName)
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("workspace: root %q: %w", root, err)
	}
	absolute = filepath.Clean(absolute)
	if absolute == sep {
		return nil, fmt.Errorf("%w: the filesystem root is not a workspace", ErrBadName)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, fmt.Errorf("workspace: root %q: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%w: root %q is not a directory", ErrBadName, root)
	}
	return &Workspace{root: absolute}, nil
}

// Root is the absolute root. It is a value, not a handle: a caller that needs a path inside the
// workspace asks Resolve, never Root plus its own join.
func (w *Workspace) Root() string { return w.root }

// Resolve returns the absolute path of a name. The name is checked first — empty, oversized, a
// control character, an absolute name, a Windows drive path, a name whose cleaned form walks
// upwards — and the joined path is checked as well, so a name that is harmless on its own but lands
// outside after cleaning is refused too.
func (w *Workspace) Resolve(name string) (string, error) {
	if err := checkName(name); err != nil {
		return "", err
	}
	clean := path.Clean(filepath.ToSlash(name))
	joined := filepath.Join(w.root, filepath.FromSlash(clean))
	if !contained(w.root, joined) {
		return "", fmt.Errorf("%w: %q", ErrOutside, name)
	}
	return joined, nil
}

// checkName is the name rule. It runs before any join, so a refused name never reaches the
// filesystem and no directory is ever created for it.
func checkName(name string) error {
	if name == "" || len(name) > 512 {
		return fmt.Errorf("%w: %d bytes", ErrBadName, len(name))
	}
	for i := 0; i < len(name); i++ {
		if name[i] < 0x20 || name[i] == 0x7f {
			return fmt.Errorf("%w: %q carries a control character", ErrBadName, name)
		}
	}
	if filepath.IsAbs(name) || strings.HasPrefix(name, "/") || strings.HasPrefix(name, `\`) {
		return fmt.Errorf("%w: %q is absolute", ErrBadName, name)
	}
	// "c:" is a drive to filepath on Windows and a plain name to filepath here; the rule refuses both
	// readings, so a statement written on one host is readable on the other.
	if len(name) >= 2 && name[1] == ':' {
		return fmt.Errorf("%w: %q looks like a drive path", ErrBadName, name)
	}
	cleaned := path.Clean(filepath.ToSlash(name))
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return fmt.Errorf("%w: %q walks upwards", ErrOutside, name)
	}
	if cleaned == "." {
		return fmt.Errorf("%w: %q is the root itself", ErrBadName, name)
	}
	return nil
}

// contained reports whether p is the root or sits under it. It is a prefix test that includes the
// separator, which is the point: "/srv/ship-copy" does not start with "/srv/ship/", so a sibling
// directory that shares the root's name is not inside the tree. The test is made on the joined path
// after filepath.Join, which has already cleaned it, so no ".." survives into it.
func contained(root, p string) bool {
	if p == root {
		return true
	}
	return strings.HasPrefix(p, root+sep)
}

// List returns the names directly under a directory name, sorted. A directory that does not exist is
// an empty list rather than an error: an operator listing the statements of a period that never closed
// reads "none", not a failure.
func (w *Workspace) List(dir string) ([]string, error) {
	target, err := w.Resolve(dir)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(target)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("workspace: list %q: %w", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		names = append(names, entry.Name())
	}
	return names, nil
}

// Exists reports whether a name resolves to a regular file. It is answered from Lstat, so a symbolic
// link named like an object is not one.
func (w *Workspace) Exists(name string) (bool, error) {
	target, err := w.Resolve(name)
	if err != nil {
		return false, err
	}
	info, err := os.Lstat(target)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("workspace: stat %q: %w", name, err)
	}
	return info.Mode().IsRegular(), nil
}
