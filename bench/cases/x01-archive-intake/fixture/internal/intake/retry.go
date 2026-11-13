package intake

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Backoff is the retry policy of a stage that talks to a device or a delivery endpoint. The archive
// itself does not retry a manifest: a delivery that failed is resumed by an operator. The policy is
// here because the scanner's sidecar sync and the export writer both use it.
type Backoff struct {
	// Base is the wait after the first failure.
	Base time.Duration
	// Max is the longest wait between two attempts.
	Max time.Duration
	// Attempts is the total number of attempts, including the first one.
	Attempts int
}

// DefaultBackoff is the policy the service uses when a caller does not set one: three attempts, a
// quarter second apart at most.
func DefaultBackoff() Backoff {
	return Backoff{Base: 50 * time.Millisecond, Max: 250 * time.Millisecond, Attempts: 3}
}

// Wait is how long the policy waits after the given attempt number (1-based).
func (b Backoff) Wait(attempt int) time.Duration {
	if b.Base <= 0 {
		return 0
	}
	wait := b.Base
	for i := 1; i < attempt; i++ {
		wait *= 2
		if b.Max > 0 && wait >= b.Max {
			return b.Max
		}
	}
	if b.Max > 0 && wait > b.Max {
		return b.Max
	}
	return wait
}

// Check refuses a policy that cannot run: fewer than one attempt, or a maximum below the base.
func (b Backoff) Check() error {
	if b.Attempts < 1 {
		return fmt.Errorf("intake: backoff needs at least one attempt")
	}
	if b.Base < 0 || b.Max < 0 {
		return fmt.Errorf("intake: backoff durations are negative")
	}
	if b.Max > 0 && b.Max < b.Base {
		return fmt.Errorf("intake: backoff maximum is below its base")
	}
	return nil
}

// Do runs fn until it succeeds or the attempts run out. The last error is returned unchanged, so a
// caller can match it; a cancelled context stops before the next attempt.
func (b Backoff) Do(ctx context.Context, fn func(attempt int) error) error {
	attempts := b.Attempts
	if attempts < 1 {
		attempts = 1
	}
	var last error
	for attempt := 1; attempt <= attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := fn(attempt); err == nil {
			return nil
		} else {
			last = err
		}
		if attempt == attempts {
			break
		}
		wait := b.Wait(attempt)
		if wait <= 0 {
			continue
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return last
}

// ErrPermanent marks an error a retry cannot fix: a manifest that does not validate, a name the tree
// refuses, a digest that does not match. A stage returns it wrapped, and the retry loop stops.
var ErrPermanent = errors.New("intake: permanent failure")

// Permanent wraps err so a caller can tell a refusal from a transient device error.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%w: %v", ErrPermanent, err)
}

// IsPermanent reports whether err is a refusal a retry would repeat.
func IsPermanent(err error) bool { return errors.Is(err, ErrPermanent) }
