package storage

import (
	"fmt"
	"strings"
)

// Quota is the space one project may use and what it uses.
type Quota struct {
	Project string
	Max     int64
	Used    int64
}

// Validate checks the quota.
func (q Quota) Validate() error {
	if strings.TrimSpace(q.Project) == "" {
		return fmt.Errorf("%w: a quota needs a project", ErrInvalid)
	}
	if q.Max <= 0 {
		return fmt.Errorf("%w: project %q may store %d bytes", ErrInvalid, q.Project, q.Max)
	}
	if q.Used < 0 {
		return fmt.Errorf("%w: project %q stores %d bytes", ErrInvalid, q.Project, q.Used)
	}
	return nil
}

// Remaining returns the bytes a project may still store.
func (q Quota) Remaining() int64 {
	rest := q.Max - q.Used
	if rest < 0 {
		return 0
	}
	return rest
}

// Fits reports whether a body of the size given fits in the quota.
func (q Quota) Fits(bytes int64) bool {
	if bytes < 0 {
		return false
	}
	return q.Used+bytes <= q.Max
}

// Exceeded reports whether the project is over its quota.
func (q Quota) Exceeded() bool { return q.Used > q.Max }

// Fraction returns the share of the quota used as a whole percentage.
func (q Quota) Fraction() int {
	if q.Max <= 0 || q.Used <= 0 {
		return 0
	}
	percent := q.Used * 100 / q.Max
	if percent > 100 {
		return 100
	}
	return int(percent)
}

// QuotaFrom builds a quota for a project from what it stores.
func QuotaFrom(project string, max, used int64) (Quota, error) {
	quota := Quota{Project: project, Max: max, Used: used}
	if err := quota.Validate(); err != nil {
		return Quota{}, err
	}
	return quota, nil
}
