package artifacts

import (
	"fmt"
	"time"

	"example.com/artifacts/internal/clock"
)

// Policy is the window an artifact is kept in.
type Policy struct {
	// Days is how long a published artifact is kept after its publication.
	Days int
	// KeepLast is the number of the project's newest artifacts that are kept whatever their age.
	KeepLast int
	// KeepStaged keeps the artifacts a job staged and never packed, which an operator turns on while
	// debugging a job.
	KeepStaged bool
}

// Validate checks the policy.
func (p Policy) Validate() error {
	if p.Days < 1 || p.Days > 3650 {
		return fmt.Errorf("%w: the retention window is %d days", ErrInvalid, p.Days)
	}
	if p.KeepLast < 1 || p.KeepLast > 1000 {
		return fmt.Errorf("%w: the policy keeps %d artifacts", ErrInvalid, p.KeepLast)
	}
	return nil
}

// Expired reports whether an artifact is past the window at an instant. An artifact that was never
// published is measured from its creation, because a pack that was never released still occupies the
// tree.
func (p Policy) Expired(record Record, at time.Time) bool {
	if record.Status == Expired {
		return true
	}
	if record.Status == Staged && p.KeepStaged {
		return false
	}
	from := record.Created
	if !record.Published.IsZero() {
		from = record.Published
	}
	horizon := clock.Day(at).AddDate(0, 0, -p.Days)
	return clock.Day(from).Before(horizon)
}

// Sweep returns the artifacts a sweep would remove: the ones past the window, except the project's
// newest KeepLast, which are kept whatever their age so a sweep cannot leave a project with nothing.
func Sweep(policy Policy, records []Record, at time.Time) ([]Record, error) {
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	byProject := make(map[string][]Record, 4)
	for _, record := range records {
		byProject[record.Project] = append(byProject[record.Project], record)
	}
	var remove []Record
	for _, group := range byProject {
		sortRecordsDescending(group)
		for i, record := range group {
			if i < policy.KeepLast {
				continue
			}
			if policy.Expired(record, at) {
				remove = append(remove, record)
			}
		}
	}
	sortRecords(remove)
	return remove, nil
}

// sortRecordsDescending orders records newest first, by publication or creation.
func sortRecordsDescending(records []Record) {
	for i := 1; i < len(records); i++ {
		for j := i; j > 0; j-- {
			if !records[j].newerThan(records[j-1]) {
				break
			}
			records[j], records[j-1] = records[j-1], records[j]
		}
	}
}

// newerThan reports whether a record is newer than another.
func (r Record) newerThan(other Record) bool {
	return r.stamp().After(other.stamp())
}

// stamp returns the instant a record is measured from.
func (r Record) stamp() time.Time {
	if !r.Published.IsZero() {
		return r.Published
	}
	return r.Created
}
