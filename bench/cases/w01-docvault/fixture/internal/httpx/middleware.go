package httpx

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"time"
)

// Middleware wraps a handler with one cross-cutting concern.
type Middleware func(http.Handler) http.Handler

// Chain applies the middleware in the order it is given: the first one runs first.
func Chain(h http.Handler, chain ...Middleware) http.Handler {
	for i := len(chain) - 1; i >= 0; i-- {
		h = chain[i](h)
	}
	return h
}

// RequestID assigns every request an id and echoes it in the response.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if id == "" {
			var buf [8]byte
			if _, err := rand.Read(buf[:]); err != nil {
				id = "unassigned"
			} else {
				id = hex.EncodeToString(buf[:])
			}
		}
		w.Header().Set("X-Request-Id", id)
		next.ServeHTTP(w, r)
	})
}

// Recover turns a panic in a handler into a 500 instead of a dropped connection.
func Recover(logger *log.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if v := recover(); v != nil {
					if logger != nil {
						logger.Printf("panic serving %s %s: %v", r.Method, r.URL.Path, v)
					}
					Fail(w, http.StatusInternalServerError, "internal error")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// AccessLog records one line per request.
func AccessLog(logger *log.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &recorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)
			if logger != nil {
				logger.Printf("%s %s %d %s", r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond))
			}
		})
	}
}

// MaxBody bounds the request body of every route, so a handler cannot be handed a size the limits
// never allowed.
func MaxBody(max int64) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, max)
			}
			next.ServeHTTP(w, r)
		})
	}
}

type recorder struct {
	http.ResponseWriter
	status int
}

func (rec *recorder) WriteHeader(status int) {
	rec.status = status
	rec.ResponseWriter.WriteHeader(status)
}

// Health is the liveness body the service answers on /healthz.
func Health(routes []string, now func() time.Time) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		WriteJSON(w, http.StatusOK, map[string]any{
			"status":  "ok",
			"routes":  len(routes),
			"served":  fmt.Sprintf("%s", now().UTC().Format(time.RFC3339)),
			"version": "docvault/1",
		})
	}
}
