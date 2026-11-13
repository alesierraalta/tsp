package staging

import (
	"fmt"
	"os"
	"path/filepath"
)

// Quota is what a staged batch currently occupies and what it is allowed to occupy. The limit is
// twice the deployment's upload cap: a batch keeps the container and the page images side by side
// while it is being built, so one upload cap would refuse a batch that is exactly at the limit.
type Quota struct {
	Used  int64
	Limit int64
}

// Used walks the batch tree and sums the file sizes. Directories are not counted; a symbolic link is
// counted as the size of the link itself, never followed, so a link cannot make a batch look larger
// or smaller than the tree it carries.
func (l Layout) Used() (int64, error) {
	var total int64
	err := filepath.Walk(l.dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			total += info.Size()
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("staging: %s is not a regular file", path)
		}
		if total > (1<<62)-info.Size() {
			return fmt.Errorf("staging: %s is larger than the quota can express", l.batch)
		}
		total += info.Size()
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return 0, fmt.Errorf("%w: %s", ErrMissing, l.batch)
		}
		return 0, fmt.Errorf("staging: measure %s: %w", l.batch, err)
	}
	return total, nil
}

// Quota measures the batch and reports how much of the deployment's batch cap it spends.
func (l Layout) Quota() (Quota, error) {
	used, err := l.Used()
	if err != nil {
		return Quota{}, err
	}
	return Quota{Used: used, Limit: l.mgr.lim.MaxUpload * 2}, nil
}

// Reserve refuses a write that would take the batch past its cap. It measures the tree before the
// write, so a caller can refuse a part that does not fit instead of writing it and failing later.
func (l Layout) Reserve(extra int64) error {
	if extra < 0 {
		return fmt.Errorf("%w: negative reservation", ErrInvalid)
	}
	quota, err := l.Quota()
	if err != nil {
		return err
	}
	if quota.Used+extra > quota.Limit {
		return fmt.Errorf("%w: batch %s would use %d of %d bytes", ErrInvalid, l.batch, quota.Used+extra, quota.Limit)
	}
	return nil
}

// Budget is what remains of the batch cap. A caller that streams a part asks once and then streams
// at most that many bytes.
func (l Layout) Budget() (int64, error) {
	quota, err := l.Quota()
	if err != nil {
		return 0, err
	}
	if quota.Used >= quota.Limit {
		return 0, nil
	}
	return quota.Limit - quota.Used, nil
}
