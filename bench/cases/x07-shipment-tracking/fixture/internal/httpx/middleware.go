package httpx

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"net/http"
	"time"
)

// Middleware wraps a handler.
type Middleware func(http.Handler) http.Handler

// Chain applies middleware to a handler, outermost first: the first middleware in the list is the one a
// request enters.
func Chain(next http.Handler, middleware ...Middleware) http.Handler {
	for i := len(middleware) - 1; i >= 0; i-- {
		next = middleware[i](next)
	}
	return next
}

// Recover turns a panic into a 500 and a log line. The panic's value is logged, never written to the
// response: a stack trace is a detail about the deployment, and a client is told that the request failed.
func Recover(logger *log.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if value := recover(); value != nil {
					if logger != nil {
						logger.Printf("panic serving %s %s: %v", r.Method, r.URL.Path, value)
					}
					JSONError(w, http.StatusInternalServerError, "internal error", "the request failed")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// WithRequestIdentifier gives every request an identifier and echoes it in the response, so a log line
// and a client's report of a failure can be matched by it.
func WithRequestIdentifier() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := newID()
			w.Header().Set("X-Request-Id", id)
			next.ServeHTTP(w, WithRequestID(r, id))
		})
	}
}

// Timing adds the server-timing header a browser's network panel reads.
func Timing(now func() time.Time) Middleware {
	if now == nil {
		now = time.Now
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := now()
			next.ServeHTTP(w, r)
			elapsed := now().Sub(start)
			w.Header().Set("Server-Timing", "app;dur="+formatMillis(elapsed))
		})
	}
}

// SecureHeaders sets the headers a response carries whatever its body: no sniffing, no framing, no
// referrer to another host.
func SecureHeaders() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("Referrer-Policy", "no-referrer")
			w.Header().Set("Cache-Control", "no-store")
			next.ServeHTTP(w, r)
		})
	}
}

// Logging writes one line per request: method, path, status and duration. The query string is not
// logged: it carries a partner's filters, and a filter is a value the log has no use for.
func Logging(logger *log.Logger, now func() time.Time) Middleware {
	if now == nil {
		now = time.Now
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := now()
			recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(recorder, r)
			if logger != nil {
				logger.Printf("%s %s %d %s request=%s", r.Method, r.URL.Path, recorder.status,
					now().Sub(start).Round(time.Millisecond), RequestIDOf(r))
			}
		})
	}
}

// statusRecorder keeps the status a handler wrote, so the log line names it.
type statusRecorder struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (s *statusRecorder) WriteHeader(status int) {
	if !s.wrote {
		s.status = status
		s.wrote = true
	}
	s.ResponseWriter.WriteHeader(status)
}

func (s *statusRecorder) Write(p []byte) (int, error) {
	if !s.wrote {
		s.wrote = true
	}
	return s.ResponseWriter.Write(p)
}

func newID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "0000000000000000"
	}
	return hex.EncodeToString(buf)
}

func formatMillis(d time.Duration) string {
	micros := d.Microseconds()
	whole := micros / 1000
	rest := micros % 1000
	return itoa(whole) + "." + pad3(rest)
}

func itoa(value int64) string {
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	var digits [20]byte
	i := len(digits)
	for value > 0 {
		i--
		digits[i] = byte('0' + value%10)
		value /= 10
	}
	if negative {
		i--
		digits[i] = '-'
	}
	return string(digits[i:])
}

func pad3(value int64) string {
	digits := [3]byte{'0', '0', '0'}
	for i := 2; i >= 0 && value > 0; i-- {
		digits[i] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[:])
}
