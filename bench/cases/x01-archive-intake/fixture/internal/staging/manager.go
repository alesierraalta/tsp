// Package staging owns the working tree of a batch: the manifest a sender delivered, the objects
// that came with it, and the page index the scanner wrote next to each scan. Nothing here is
// committed; the intake pipeline reads this tree and the store keeps what survives it.
package staging

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"example.com/archive/internal/clock"
	"example.com/archive/internal/config"
	"example.com/archive/internal/idgen"
	"example.com/archive/internal/storepath"
)

// Errors a staging operation returns. A caller maps ErrOutside to a refusal and ErrMissing to a 404.
var (
	ErrMissing = errors.New("staging: missing")
	ErrOutside = storepath.ErrOutside
	ErrInvalid = storepath.ErrInvalid
	ErrExists  = errors.New("staging: batch already staged")
)

// Manager owns the staging tree of one deployment. Every layout it hands out lives under the
// deployment's root, and every file inside a layout is resolved through storepath.
type Manager struct {
	root  string
	base  string
	lim   config.Limits
	clock clock.Clock
}

// Open prepares the staging tree under root. The directory is created when it is absent; a root that
// is not absolute is refused, because a relative root would make the containment comparison depend on
// the process's working directory.
func Open(root string, lim config.Limits, now clock.Clock) (*Manager, error) {
	if root == "" {
		return nil, fmt.Errorf("staging: no root")
	}
	if !filepath.IsAbs(root) {
		return nil, fmt.Errorf("staging: root %q is not absolute", root)
	}
	if err := lim.Or().Check(); err != nil {
		return nil, err
	}
	clean := filepath.Clean(root)
	base := filepath.Join(clean, ".staging")
	if err := os.MkdirAll(base, 0o755); err != nil {
		return nil, fmt.Errorf("staging: create %s: %w", base, err)
	}
	return &Manager{root: clean, base: base, lim: lim.Or(), clock: now}, nil
}

// Root is the deployment root the staging tree lives under.
func (m *Manager) Root() string { return m.root }

// Base is the staging directory itself.
func (m *Manager) Base() string { return m.base }

// Limits are the caps this manager enforces on a layout.
func (m *Manager) Limits() config.Limits { return m.lim }

// NewBatchID mints an id for a batch that arrived without one.
func (m *Manager) NewBatchID() string { return idgen.New(idgen.Batch) }

// Layout returns the working tree of one batch. The id is validated before it becomes a directory
// name, so a caller cannot point a layout at an arbitrary path.
func (m *Manager) Layout(batchID string) (Layout, error) {
	if !idgen.Valid(idgen.Batch, batchID) {
		return Layout{}, fmt.Errorf("%w: batch id %q", ErrInvalid, batchID)
	}
	dir := filepath.Join(m.base, batchID)
	if !storepath.Contained(m.base, dir) {
		return Layout{}, fmt.Errorf("%w: batch id %q", ErrOutside, batchID)
	}
	return Layout{mgr: m, batch: batchID, dir: dir}, nil
}

// Layouts lists the batches currently staged, most recent first. A directory whose name is not a
// batch id is skipped rather than read, so a stray file in the staging directory is not a batch.
func (m *Manager) Layouts() ([]Layout, error) {
	entries, err := os.ReadDir(m.base)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("staging: list: %w", err)
	}
	var out []Layout
	for _, entry := range entries {
		if !entry.IsDir() || !idgen.Valid(idgen.Batch, entry.Name()) {
			continue
		}
		out = append(out, Layout{mgr: m, batch: entry.Name(), dir: filepath.Join(m.base, entry.Name())})
	}
	return out, nil
}

// Used is the number of bytes every staged batch occupies together, for the operator page.
func (m *Manager) Used() (int64, error) {
	layouts, err := m.Layouts()
	if err != nil {
		return 0, err
	}
	var total int64
	for _, l := range layouts {
		used, err := l.Used()
		if err != nil {
			return total, err
		}
		total += used
	}
	return total, nil
}

// Sweep removes the layouts whose batch nobody is working on: a staged batch older than the given
// window is discarded. The decision is made on the batch directory's own timestamp, read in UTC.
func (m *Manager) Sweep(olderThan clockTime) ([]string, error) {
	layouts, err := m.Layouts()
	if err != nil {
		return nil, err
	}
	now := clock.Now(m.clock)
	var removed []string
	for _, l := range layouts {
		info, err := os.Stat(l.dir)
		if err != nil {
			continue
		}
		if info.ModTime().UTC().Before(now.Add(-olderThan)) {
			if err := l.Remove(); err != nil {
				return removed, err
			}
			removed = append(removed, l.batch)
		}
	}
	return removed, nil
}
