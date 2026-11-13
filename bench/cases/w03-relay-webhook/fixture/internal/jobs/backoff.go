package jobs

import (
	"time"
)

// Backoff is the wait sequence between attempts: the first wait, doubled per attempt, capped at the
// maximum and never above the deadline the caller has.
type Backoff struct {
	First time.Duration
	Max   time.Duration
}

// DefaultBackoff is the sequence the queue uses.
func DefaultBackoff() Backoff {
	return Backoff{First: 250 * time.Millisecond, Max: 30 * time.Second}
}

// At returns the wait after the attempt number given, counting from one. An attempt below one has
// no wait: nothing has failed yet.
func (b Backoff) At(attempt int) time.Duration {
	if attempt < 1 {
		return 0
	}
	first := b.First
	if first <= 0 {
		first = 250 * time.Millisecond
	}
	max := b.Max
	if max <= 0 {
		max = 30 * time.Second
	}
	wait := first
	for i := 1; i < attempt; i++ {
		if wait >= max/2 {
			return max
		}
		wait *= 2
	}
	if wait > max {
		return max
	}
	return wait
}

// Bounded returns the wait after an attempt, clamped to a deadline: a wait that would end after the
// deadline is not a wait the queue can afford, so the attempt is not made.
func (b Backoff) Bounded(attempt int, deadline time.Time, now time.Time) (time.Duration, bool) {
	wait := b.At(attempt)
	if wait == 0 {
		return 0, false
	}
	if !now.Add(wait).Before(deadline) {
		return 0, false
	}
	return wait, true
}

// Sequence renders the first waits, which is what the operator documentation prints.
func (b Backoff) Sequence(n int) []time.Duration {
	if n <= 0 {
		return nil
	}
	out := make([]time.Duration, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, b.At(i))
	}
	return out
}
