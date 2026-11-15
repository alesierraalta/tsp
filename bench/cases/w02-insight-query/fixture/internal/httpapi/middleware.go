package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// Middleware wraps a handler with one cross-cutting concern. The service uses them in the order
// Handler applies them, so a request is identified before it is timed and timed before it is served.
type Middleware func(http.Handler) http.Handler

// Chain applies the middleware in the order given: the first one runs first.
func Chain(h http.Handler, chain ...Middleware) http.Handler {
	for i := len(chain) - 1; i >= 0; i-- {
		h = chain[i](h)
	}
	return h
}

// WithTimeout bounds the time a handler may take. A request that runs past the deadline is answered
// with 504 instead of holding the connection until the client gives up.
func WithTimeout(limit time.Duration) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := timeoutContext(r, limit)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// timeoutContext is a value the package can replace in a test without waiting for a real deadline.
var timeoutContext = func(r *http.Request, limit time.Duration) (context.Context, context.CancelFunc) {
	c, cancel := contextWithTimeout(r, limit)
	return c, cancel
}

// MaxHeaderBytes refuses a request whose headers exceed the limit before the handler reads them.
func MaxHeaderBytes(limit int) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if limit > 0 {
				size := 0
				for name, values := range r.Header {
					size += len(name)
					for _, value := range values {
						size += len(value)
					}
				}
				if size > limit {
					fail(w, http.StatusRequestHeaderFieldsTooLarge,
						fmt.Sprintf("headers above %d bytes", limit))
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// NoStore stops a response from being cached: a report is a value of the store at the moment it ran,
// and a cached copy would be served under a newer timestamp.
func NoStore(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
