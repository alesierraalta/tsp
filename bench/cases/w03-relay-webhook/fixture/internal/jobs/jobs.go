// Package jobs runs the deliveries the service owes, one queue at a time.
package jobs

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"example.com/relay/internal/notify"
)

// Job is one pending delivery.
type Job struct {
	ID           string
	Subscription notify.Subscription
	Event        notify.Event
	Attempts     int
	LastError    string
	Done         bool
}

// Runner owns the queue and delivers it.
type Runner struct {
	mu       sync.Mutex
	notifier *notify.Notifier
	queue    []*Job
	now      func() time.Time
}

// New returns an empty runner over a notifier.
func New(notifier *notify.Notifier) *Runner {
	return &Runner{notifier: notifier, now: func() time.Time { return time.Now().UTC() }}
}

// SetClock replaces the clock the queue stamps jobs with.
func (r *Runner) SetClock(now func() time.Time) {
	if now != nil {
		r.now = now
	}
}

// Enqueue adds one delivery. A subscription that does not ask for the event is refused at enqueue
// time, so the queue never carries a delivery nobody wants.
func (r *Runner) Enqueue(sub notify.Subscription, ev notify.Event) (string, error) {
	if err := sub.Validate(); err != nil {
		return "", err
	}
	if !sub.Wants(ev.Kind) {
		return "", fmt.Errorf("%w: %q does not ask for %q", notify.ErrInvalid, sub.ID, ev.Kind)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	id := fmt.Sprintf("job-%d", len(r.queue)+1)
	r.queue = append(r.queue, &Job{ID: id, Subscription: sub, Event: ev})
	return id, nil
}

// Run delivers every pending job once, in enqueue order, and returns what happened. A job the
// endpoint accepted is done; a job that failed stays in the queue with its attempt count.
func (r *Runner) Run(ctx context.Context) ([]Job, error) {
	r.mu.Lock()
	pending := make([]*Job, len(r.queue))
	copy(pending, r.queue)
	r.mu.Unlock()

	results := make([]Job, 0, len(pending))
	var firstErr error
	for _, job := range pending {
		if job.Done {
			continue
		}
		delivery, err := r.notifier.Deliver(ctx, job.Subscription, job.Event)
		job.Attempts += delivery.Attempts
		if err != nil {
			job.LastError = err.Error()
			if firstErr == nil {
				firstErr = err
			}
		} else {
			job.Done = delivery.Delivered
			job.LastError = ""
		}
		results = append(results, *job)
	}
	return results, firstErr
}

// Pending reports how many jobs are still owed.
func (r *Runner) Pending() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, job := range r.queue {
		if !job.Done {
			n++
		}
	}
	return n
}

// Status lists the queue in enqueue order.
func (r *Runner) Status() []Job {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]Job, 0, len(r.queue))
	for _, job := range r.queue {
		out = append(out, *job)
	}
	return out
}

// Retryable reports whether a delivery error is one another attempt could fix. A blocked endpoint
// is not: retrying it would only repeat the same refusal.
func Retryable(err error) bool {
	if err == nil {
		return false
	}
	return !containsBlocked(err)
}

func containsBlocked(err error) bool {
	for err != nil {
		if err == notify.ErrBlocked {
			return true
		}
		unwrapper, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = unwrapper.Unwrap()
	}
	return false
}

// KindsInQueue lists the event kinds the queue carries, sorted.
func (r *Runner) KindsInQueue() []string {
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
