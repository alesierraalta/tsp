package staging

import (
	"example.com/archive/internal/clock"
	"example.com/archive/internal/storepath"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Files lists the names of every file inside the batch tree, relative to it, for the operator page's
// "what is staged" block. The walk refuses to leave the tree.
func (l Layout) Files() ([]string, error) {
	var names []string
	err := filepath.Walk(l.dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := storepath.Key(l.dir, path)
		if err != nil {
			return err
		}
		names = append(names, rel)
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrMissing, l.batch)
		}
		return nil, fmt.Errorf("staging: walk: %w", err)
	}
	sort.Strings(names)
	return names, nil
}

// MarkCommitted records that the batch finished. A batch that is marked twice stays committed.
func (l Layout) MarkCommitted(at time.Time) error {
	target, err := l.resolved(CommittedFile)
	if err != nil {
		return err
	}
	return os.WriteFile(target, []byte(clock.Stamp(at)+"\n"), 0o644)
}

// CommittedAt reports whether the batch finished and when.
func (l Layout) CommittedAt() (time.Time, bool) {
	target, err := l.resolved(CommittedFile)
	if err != nil {
		return time.Time{}, false
	}
	body, err := os.ReadFile(target)
	if err != nil {
		return time.Time{}, false
	}
	at, err := clock.ParseRFC3339(string(trimNewline(body)))
	if err != nil {
		return time.Time{}, false
	}
	return at, true
}

// Remove deletes the batch's working tree. It refuses to remove anything but a directory it owns:
// the path is checked to sit directly under the staging base and to carry a batch id.
func (l Layout) Remove() error {
	if filepath.Dir(l.dir) != l.mgr.base || !storepath.Contained(l.mgr.base, l.dir) {
		return fmt.Errorf("%w: %s is not a staged batch", ErrInvalid, l.dir)
	}
	if err := os.RemoveAll(l.dir); err != nil {
		return fmt.Errorf("staging: remove %s: %w", l.batch, err)
	}
	return nil
}

// read reads one file of the tree through the containment rule.
func (l Layout) read(rel string) ([]byte, error) {
	target, err := l.resolved(rel)
	if err != nil {
		return nil, err
	}
	body, err := os.ReadFile(target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrMissing, rel)
		}
		return nil, fmt.Errorf("staging: read %s: %w", rel, err)
	}
	return body, nil
}

// write writes one file of the tree through the containment rule, creating its directory.
func (l Layout) write(rel string, data []byte) error {
	target, err := l.resolved(rel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("staging: create %s: %w", filepath.Dir(target), err)
	}
	if err := os.WriteFile(target, data, 0o644); err != nil {
		return fmt.Errorf("staging: write %s: %w", rel, err)
	}
	return nil
}

// trimNewline removes the single trailing newline a marker file carries.
func trimNewline(body []byte) []byte {
	for len(body) > 0 && (body[len(body)-1] == '\n' || body[len(body)-1] == '\r') {
		body = body[:len(body)-1]
	}
	return body
}
