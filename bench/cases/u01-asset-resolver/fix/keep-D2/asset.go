// Package assets maps browser request paths and build-manifest bundle entries onto one root directory.
package assets

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// ErrOutside reports a path that would resolve outside the resolver's root.
var ErrOutside = errors.New("assets: path leaves the root")

// ErrEmpty reports an empty request path or manifest target.
var ErrEmpty = errors.New("assets: empty path")

// ErrNoBundle reports a bundle name the manifest does not carry.
var ErrNoBundle = errors.New("assets: no such bundle")

// Resolver maps request paths and manifest entries onto a root directory.
type Resolver struct {
	root     string
	manifest map[string]string
}

// New returns a resolver for root, using manifest to resolve bundle names. root must be absolute and is
// cleaned; the filesystem root is refused so a missing configuration cannot serve the whole disk.
func New(root string, manifest map[string]string) (*Resolver, error) {
	if strings.TrimSpace(root) == "" {
		return nil, ErrEmpty
	}
	if !filepath.IsAbs(root) {
		return nil, fmt.Errorf("assets: root %q is not absolute", root)
	}
	clean := filepath.Clean(root)
	if clean == string(filepath.Separator) {
		return nil, fmt.Errorf("assets: root %q is the filesystem root", root)
	}
	r := &Resolver{root: clean, manifest: make(map[string]string, len(manifest))}
	for name, target := range manifest {
		if strings.TrimSpace(name) == "" {
			return nil, ErrEmpty
		}
		r.manifest[name] = target
	}
	return r, nil
}

// Root is the directory every returned path resolves inside.
func (r *Resolver) Root() string { return r.root }

// inside reports whether p stays within the root. The separator makes a sibling directory that merely shares
// the root's prefix fail the comparison.
func (r *Resolver) inside(p string) bool {
	return p == r.root || strings.HasPrefix(p, r.root+string(filepath.Separator))
}

// Decode percent-decodes a request path. Malformed encoding is reported instead of passing through, because
// a half-decoded path is neither the path the browser asked for nor the path on disk.
func Decode(requestPath string) (string, error) {
	decoded, err := url.PathUnescape(requestPath)
	if err != nil {
		return "", fmt.Errorf("assets: percent-decode %q: %w", requestPath, err)
	}
	return decoded, nil
}

// Resolve returns the absolute path of a request path, or ErrOutside when it would leave the root.
func (r *Resolver) Resolve(requestPath string) (string, error) {
	if requestPath == "" {
		return "", ErrEmpty
	}
	if strings.ContainsRune(requestPath, 0) {
		return "", ErrOutside
	}
	decoded, err := Decode(requestPath)
	if err != nil {
		return "", err
	}
	if path.IsAbs(decoded) {
		return "", ErrOutside
	}
	joined := filepath.Join(r.root, filepath.FromSlash(decoded))
	if !r.inside(joined) {
		return "", ErrOutside
	}
	return joined, nil
}

// Bundle returns the absolute path of a bundle from the manifest.
func (r *Resolver) Bundle(name string) (string, error) {
	target, ok := r.manifest[name]
	if !ok {
		return "", fmt.Errorf("%w: %q", ErrNoBundle, name)
	}
	if target == "" {
		return "", ErrEmpty
	}
	return filepath.Join(r.root, filepath.FromSlash(target)), nil
}

// Key returns the cache key of a request path: the path relative to the root, with forward slashes.
func (r *Resolver) Key(requestPath string) string {
	resolved, err := r.Resolve(requestPath)
	if err != nil {
		return ""
	}
	rel, err := filepath.Rel(r.root, resolved)
	if err != nil {
		return ""
	}
	return filepath.ToSlash(rel)
}

// Manifest lists the bundle names in sorted order.
func (r *Resolver) Manifest() []string {
	names := make([]string, 0, len(r.manifest))
	for name := range r.manifest {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// Reload replaces the manifest, for a build that re-emits it without restarting the server.
func (r *Resolver) Reload(manifest map[string]string) error {
	next := make(map[string]string, len(manifest))
	for name, target := range manifest {
		if strings.TrimSpace(name) == "" {
			return ErrEmpty
		}
		next[name] = target
	}
	r.manifest = next
	return nil
}
