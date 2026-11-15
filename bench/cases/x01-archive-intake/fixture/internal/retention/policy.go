// Package retention decides which closed batches and objects a sweep may remove. Every decision is
// made on whole UTC days, so a batch does not become old at a time of day.
package retention

import (
	"fmt"
	"sort"
	"time"

	"example.com/archive/internal/clock"
)

// Policy is the window a sweep applies: how many days a closed batch is kept, and how many closed
// batches are kept whatever their age.
type Policy struct {
	// KeepDays is the retention window in whole days.
	KeepDays int
	// KeepBatches is the number of most recent closed batches that are never swept.
	KeepBatches int
	// KeepPinned marks batches whose manifest asked to be kept. A pinned batch is never swept.
	KeepPinned bool
}

// Candidate is one closed batch a sweep may remove.
type Candidate struct {
	// ID is the batch identifier; it is what a verdict names.
	ID string
	// Closed is the instant the batch closed.
	Closed time.Time
	// Pinned marks a batch the manifest asked to keep.
	Pinned bool
}

// Verdict is the sweep's decision about one candidate and the reason it gives.
type Verdict struct {
	ID     string
	Remove bool
	Reason string
}

// Sweep decides every candidate. The rule, in order: a pinned batch stays; the most recent
// KeepBatches closed batches stay; a batch whose UTC day is older than the horizon is removed; every
// other batch stays. The order matters because a young batch cannot be removed by age and an old
// batch cannot be saved by being recent.
func Sweep(candidates []Candidate, p Policy, now time.Time) []Verdict {
	horizon := clock.AddDays(now, -p.KeepDays)
	sorted := append([]Candidate(nil), candidates...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Closed.Equal(sorted[j].Closed) {
			return sorted[i].ID < sorted[j].ID
		}
		return sorted[i].Closed.After(sorted[j].Closed)
	})
	recent := map[string]bool{}
	if p.KeepBatches > 0 {
		for i, c := range sorted {
			if i >= p.KeepBatches {
				break
			}
			recent[c.ID] = true
		}
	}
	verdicts := make([]Verdict, 0, len(sorted))
	for _, c := range sorted {
		switch {
		case c.Pinned && p.KeepPinned:
			verdicts = append(verdicts, Verdict{ID: c.ID, Reason: "pinned by its manifest"})
		case recent[c.ID]:
			verdicts = append(verdicts, Verdict{ID: c.ID, Reason: fmt.Sprintf("one of the last %d batches", p.KeepBatches)})
		case clock.Day(c.Closed).Before(horizon):
			verdicts = append(verdicts, Verdict{
				ID:     c.ID,
				Remove: true,
				Reason: fmt.Sprintf("closed %s, before the %d-day horizon %s", clock.Day(c.Closed).Format("2006-01-02"), p.KeepDays, horizon.Format("2006-01-02")),
			})
		default:
			verdicts = append(verdicts, Verdict{ID: c.ID, Reason: "inside the retention window"})
		}
	}
	return verdicts
}

// Horizon is the oldest UTC day a batch may carry and still be kept under p at now. A page prints it
// next to the retention setting so an operator sees what the next sweep will remove.
func Horizon(p Policy, now time.Time) time.Time { return clock.AddDays(now, -p.KeepDays) }

// Expired reports whether one candidate is past the policy's window, ignoring pinning and recency.
// It is the helper a single-object cleanup uses; a batch sweep uses Sweep, which knows both.
func Expired(closed, now time.Time, days int) bool {
	return clock.Day(closed).Before(clock.AddDays(now, -days))
}

// Empty reports whether a policy keeps everything: no day window and no recency rule. A deployment
// that sets neither runs no sweeps at all.
func (p Policy) Empty() bool { return p.KeepDays <= 0 && p.KeepBatches <= 0 }

// Check refuses a policy that would keep every batch by accident: a negative window is a typo, not
// an intention.
func (p Policy) Check() error {
	if p.KeepDays < 0 {
		return fmt.Errorf("retention: negative day window %d", p.KeepDays)
	}
	if p.KeepBatches < 0 {
		return fmt.Errorf("retention: negative batch window %d", p.KeepBatches)
	}
	return nil
}

// Describe renders the policy for the operator page.
func (p Policy) Describe() string {
	if p.Empty() {
		return "retention is off"
	}
	return fmt.Sprintf("keep %d days and the last %d batches", p.KeepDays, p.KeepBatches)
}
