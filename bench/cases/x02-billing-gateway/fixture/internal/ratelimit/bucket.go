// Package ratelimit bounds how often one subject may call the gateway. A bucket refills over time
// rather than resetting on a boundary, so a burst at the end of one window does not meet a fresh
// allowance at the start of the next.
package ratelimit

import (
	"fmt"
	"sync"
	"time"

	"example.com/billing/internal/clock"
)

// Bucket is one subject's allowance.
type Bucket struct {
	tokens   float64
	updated  time.Time
	capacity float64
	refill   float64 // tokens per second
}

// Limiter is a set of buckets, one per subject.
type Limiter struct {
	mu       sync.Mutex
	now      clock.Clock
	buckets  map[string]*Bucket
	capacity float64
	refill   float64
	window   time.Duration
}

// New returns a limiter that gives a subject capacity requests per window.
func New(capacity int, window time.Duration, now clock.Clock) *Limiter {
	if capacity <= 0 {
		capacity = 60
	}
	if window <= 0 {
		window = time.Minute
	}
	if now == nil {
		now = clock.System()
	}
	return &Limiter{
		now:      now,
		buckets:  make(map[string]*Bucket),
		capacity: float64(capacity),
		refill:   float64(capacity) / window.Seconds(),
		window:   window,
	}
}

// SetClock replaces the clock the buckets refill against.
func (l *Limiter) SetClock(now clock.Clock) {
	if now == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.now = now
}

// Allow takes one token from a subject's bucket and reports whether it had one. A subject seen for the
// first time starts with a full bucket.
func (l *Limiter) Allow(subject string) (bool, time.Duration) {
	if subject == "" {
		subject = "(anonymous)"
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now().UTC()
	bucket, ok := l.buckets[subject]
	if !ok {
		bucket = &Bucket{tokens: l.capacity, updated: now, capacity: l.capacity, refill: l.refill}
		l.buckets[subject] = bucket
	}
	l.refillBucket(bucket, now)
	if bucket.tokens < 1 {
		wait := time.Duration((1 - bucket.tokens) / bucket.refill * float64(time.Second))
		if wait < time.Second {
			wait = time.Second
		}
		return false, wait
	}
	bucket.tokens--
	return true, 0
}

// refillBucket adds the tokens that elapsed since the bucket was last touched.
func (l *Limiter) refillBucket(bucket *Bucket, now time.Time) {
	if now.After(bucket.updated) {
		elapsed := now.Sub(bucket.updated).Seconds()
		bucket.tokens += elapsed * bucket.refill
		if bucket.tokens > bucket.capacity {
			bucket.tokens = bucket.capacity
		}
		bucket.updated = now
	}
}

// Remaining reports how many requests a subject has left without taking one.
func (l *Limiter) Remaining(subject string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now().UTC()
	bucket, ok := l.buckets[subject]
	if !ok {
		return int(l.capacity)
	}
	l.refillBucket(bucket, now)
	if bucket.tokens < 0 {
		return 0
	}
	return int(bucket.tokens)
}

// Reset forgets every bucket, which an operator does after fixing a client that exhausted its
// allowance by looping.
func (l *Limiter) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.buckets = make(map[string]*Bucket)
}

// String renders the limiter's shape for the console page.
func (l *Limiter) String() string {
	return fmt.Sprintf("%.0f requests per %s", l.capacity, l.window)
}
