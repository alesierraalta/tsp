package retention

import (
	"fmt"
	"sort"
	"time"

	"example.com/archive/internal/clock"
	"example.com/archive/internal/store"
)

// Object is one committed object a sweep may delete: its id, the tenant, the batch it came from and the
// instant it committed.
type Object struct {
	ID        string
	Tenant    string
	Batch     string
	Committed time.Time
}

// Objects turns a record set into the candidates an object sweep reads. A record with no commit instant
// is left out rather than treated as ancient: an object whose age is unknown is not deleted on a guess.
func Objects(records []store.Record) []Object {
	out := make([]Object, 0, len(records))
	for _, record := range records {
		at := record.CommittedAt()
		if at.IsZero() {
			continue
		}
		out = append(out, Object{ID: record.ID, Tenant: record.Tenant, Batch: record.Batch, Committed: at})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Committed.Equal(out[j].Committed) {
			return out[i].ID < out[j].ID
		}
		return out[i].Committed.Before(out[j].Committed)
	})
	return out
}

// ObjectVerdict is the sweep's decision about one object.
type ObjectVerdict struct {
	ID     string
	Batch  string
	Remove bool
	Reason string
}

// SweepObjects decides every object under the policy's day window. Unlike a batch sweep there is no
// recency rule: an object is kept by being young, and the batch sweep protects the batches that carry one.
func SweepObjects(objects []Object, p Policy, now time.Time) []ObjectVerdict {
	horizon := clock.AddDays(now, -p.KeepDays)
	verdicts := make([]ObjectVerdict, 0, len(objects))
	for _, object := range objects {
		if p.KeepDays <= 0 {
			verdicts = append(verdicts, ObjectVerdict{ID: object.ID, Batch: object.Batch, Reason: "retention is off"})
			continue
		}
		if clock.Day(object.Committed).Before(horizon) {
			verdicts = append(verdicts, ObjectVerdict{
				ID:     object.ID,
				Batch:  object.Batch,
				Remove: true,
				Reason: fmt.Sprintf("committed %s, before the horizon %s", clock.Day(object.Committed).Format("2006-01-02"), horizon.Format("2006-01-02")),
			})
			continue
		}
		verdicts = append(verdicts, ObjectVerdict{ID: object.ID, Batch: object.Batch, Reason: "inside the retention window"})
	}
	return verdicts
}

// Removable filters a batch sweep down to the batches whose objects may actually be deleted: a batch is
// only removed when every object it carries is inside the window's decision, because deleting the tree of
// a batch whose objects stay would leave objects nothing can explain.
func Removable(batchVerdicts []Verdict, objectVerdicts []ObjectVerdict) []string {
	protected := map[string]bool{}
	for _, verdict := range objectVerdicts {
		if !verdict.Remove {
			protected[verdict.Batch] = true
		}
	}
	var batches []string
	for _, verdict := range batchVerdicts {
		if !verdict.Remove || protected[verdict.ID] {
			continue
		}
		batches = append(batches, verdict.ID)
	}
	sort.Strings(batches)
	return batches
}

// Window is the day range a sweep reads at now.
func (p Policy) Window(now time.Time) (clock.Window, error) {
	window := clock.DaysWindow(now, p.KeepDays)
	if err := window.Check(); err != nil {
		return clock.Window{}, err
	}
	return window, nil
}
