package httpapi

import (
	"context"
	"net/http"
	"time"
)

// contextWithTimeout is the one call the middleware makes, kept here so the file that names the
// deadine does not import context twice.
func contextWithTimeout(r *http.Request, limit time.Duration) (context.Context, func()) {
	if limit <= 0 {
		return r.Context(), func() {}
	}
	return context.WithTimeout(r.Context(), limit)
}

// Apply replaces the store's rows with the ones a deployment loaded. It is the seam an operator
// uses after a billing export, and it is a value so a test can drive it.
type Apply func() error

// Reloader reports whether the service has reloaded since start-up and how long the last reload
// took. A report served from a half-loaded store would answer a question nobody asked, so the
// handler that reads it checks the flag first.
type Reloader struct {
	done  bool
	last  time.Duration
	apply Apply
}

// NewReloader wires a reload function.
func NewReloader(apply Apply) *Reloader { return &Reloader{apply: apply} }

// Run applies the reload and records how long it took.
func (r *Reloader) Run() error {
	start := time.Now()
	if r.apply != nil {
		if err := r.apply(); err != nil {
			return err
		}
	}
	r.last = time.Since(start)
	r.done = true
	return nil
}

// Done reports whether a reload has completed.
func (r *Reloader) Done() bool { return r.done }

// Last reports how long the last reload took.
func (r *Reloader) Last() time.Duration { return r.last }
