package httpx

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Middleware wraps a handler.
type Middleware func(http.Handler) http.Handler

// Chain applies the middleware in the order given: the first one is the outermost, so it sees the
// request before every other one.
func Chain(h http.Handler, mws ...Middleware) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

// Recoverer turns a panic into a 500 and logs it. Without it a panic in one handler takes the whole
// process down, including the batches other tenants are importing.
func Recoverer(logger *log.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					if logger != nil {
						logger.Printf("panic serving %s %s: %v", r.Method, r.URL.Path, recovered)
					}
					Fail(w, http.StatusInternalServerError, "internal error")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// Logger writes one line per request: method, path, status and how long it took. The path is logged
// without its query, because a query may carry a name the log has no business keeping.
func Logger(logger *log.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(recorder, r)
			if logger != nil {
				logger.Printf("%s %s %d %s", r.Method, r.URL.Path, recorder.status, time.Since(start).Round(time.Millisecond))
			}
		})
	}
}

// statusRecorder remembers the status a handler wrote, for the log line and for a caller that checks
// what a nested handler produced.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

// WriteHeader records the status before writing it.
func (r *statusRecorder) WriteHeader(status int) {
	if r.wroteHeader {
		return
	}
	r.status = status
	r.wroteHeader = true
	r.ResponseWriter.WriteHeader(status)
}

// Write records an implicit 200 on the first body byte.
func (r *statusRecorder) Write(body []byte) (int, error) {
	if !r.wroteHeader {
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(body)
}

// Status is the status the handler wrote.
func (r *statusRecorder) Status() int { return r.status }

// SecurityHeaders sets the response headers a browser needs to treat a page as text: no sniffing, no
// framing, and no referrer carrying a batch name to another site.
func SecurityHeaders() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("Referrer-Policy", "no-referrer")
			next.ServeHTTP(w, r)
		})
	}
}

// MaxBody refuses a request whose declared body is larger than the cap before the handler reads a
// byte of it. A request with no Content-Length is left to the bounded reader inside the handler.
func MaxBody(cap int64) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > cap {
				Fail(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("body is longer than %d bytes", cap))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Query reads one query value, trimmed. It is the only way a handler reads a query parameter, so the
// trimming rule is applied once.
func Query(r *http.Request, key string) string {
	return strings.TrimSpace(r.URL.Query().Get(key))
}

// IntQuery reads an integer query value with a default and a cap. A value above the cap is refused
// rather than clamped: a caller that asked for more than the service serves is told, not silently
// given the cap.
func IntQuery(r *http.Request, key string, def, max int) (int, error) {
	raw := Query(r, key)
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s=%q is not a number", key, raw)
	}
	if n < 0 {
		return 0, fmt.Errorf("%s=%q is negative", key, raw)
	}
	if max > 0 && n > max {
		return 0, fmt.Errorf("%s=%d is more than the %d this endpoint serves", key, n, max)
	}
	return n, nil
}

// BoolQuery reads a flag query value: "1", "true" or "yes" are true, everything else is false.
func BoolQuery(r *http.Request, key string) bool {
	switch strings.ToLower(Query(r, key)) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

// WantsHTML reports whether the client asked for a page rather than for JSON. Browsers send the
// header; a script that wants JSON sends "application/json" or nothing.
func WantsHTML(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "text/html")
}
