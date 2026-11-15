package jobs

import (
	"sort"
	"time"
)

// State is the lifecycle of one job as an operator reads it.
type State string

// The states a job passes through.
const (
	StateQueued     State = "queued"
	StateDelivering State = "delivering"
	StateDone       State = "done"
	StateFailed     State = "failed"
)

// Snapshot is the queue as one value: the counts per state and the oldest job still owed.
type Snapshot struct {
	Queued   int
	Done     int
	Failed   int
	Oldest   time.Time
	Attempts int
}

// Snapshot renders the queue.
func (r *Runner) Snapshot() Snapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	var snapshot Snapshot
	for _, job := range r.queue {
		switch {
		case job.Done:
			snapshot.Done++
		case job.LastError != "":
			snapshot.Failed++
			snapshot.Queued++
		default:
			snapshot.Queued++
		}
		snapshot.Attempts += job.Attempts
		if !job.Done && (snapshot.Oldest.IsZero() || job.Event.At.Before(snapshot.Oldest)) {
			snapshot.Oldest = job.Event.At
		}
	}
	return snapshot
}

// State names the state of one job.
func (j Job) State() State {
	switch {
	case j.Done:
		return StateDone
	case j.LastError != "":
		return StateFailed
	default:
		return StateQueued
	}
}

// Kinds lists the kinds a runner's queue carries, sorted, without taking the lock twice.
func (r *Runner) Kinds() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	seen := map[string]bool{}
	for _, job := range r.queue {
		seen[job.Event.Kind] = true
	}
	out := make([]string, 0, len(seen))
	for kind := range seen {
		out = append(out, kind)
	}
	sort.Strings(out)
	return out
}

// Due returns the jobs whose event happened before cutoff and that are still owed, in queue order.
// The bound is a time, not a count, so a queue drained late delivers everything it owes in one run.
func (r *Runner) Due(cutoff time.Time) []Job {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Job
	for _, job := range r.queue {
		if job.Done || !job.Event.At.Before(cutoff) {
			continue
		}
		out = append(out, *job)
	}
	return out
}
