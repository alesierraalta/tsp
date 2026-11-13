// Package httpx is the small HTTP layer of the service: the response writers, the bounded body
// decoder every handler uses, and the middleware every request passes through.
package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

// ErrBodyTooLarge is returned by DecodeJSON for a body above the bound.
var ErrBodyTooLarge = errors.New("http: request body is larger than the bound")

// WriteJSON writes a JSON response.
func WriteJSON(w http.ResponseWriter, status int, value any) {
	body, err := json.Marshal(value)
	if err != nil {
		Fail(w, http.StatusInternalServerError, "cannot encode response")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	w.Write(body)
	w.Write([]byte("\n"))
}

// WriteHTML writes an HTML response. The body is written as it is: every fragment comes from package
// render, which escapes the values it interpolates.
func WriteHTML(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	w.Write([]byte(body))
}

// WriteCSV writes a CSV response.
func WriteCSV(w http.ResponseWriter, status int, name string, body []byte) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+name+"\"")
	w.WriteHeader(status)
	w.Write(body)
}

// WriteBytes writes a binary response.
func WriteBytes(w http.ResponseWriter, mediaType string, body []byte) {
	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	w.Write(body)
}

// Fail writes the standard error body.
func Fail(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, struct {
		Error string `json:"error"`
	}{Error: message})
}

// DecodeJSON reads a JSON request body of at most max bytes into into. The reader is bounded by
// MaxBytesReader, so a body that is too large is refused rather than truncated into something the
// caller did not send, and a second value after the object is refused as well.
func DecodeJSON(w http.ResponseWriter, r *http.Request, max int64, into any) error {
	if r.Body == nil {
		return fmt.Errorf("http: empty body")
	}
	limited := http.MaxBytesReader(w, r.Body, max)
	decoder := json.NewDecoder(limited)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return fmt.Errorf("%w: %d bytes", ErrBodyTooLarge, max)
		}
		return fmt.Errorf("http: decode body: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("http: body carries more than one value")
	}
	return nil
}

// QueryInt reads an integer query parameter, returning fallback when it is absent.
func QueryInt(r *http.Request, name string, fallback int) (int, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("http: %s=%q is not an integer", name, raw)
	}
	return value, nil
}

// Requests is the middleware every request passes through: it bounds the time a handler may take,
// answers a panic with a refusal instead of a stack trace, and logs one line per request.
func Requests(logger func(format string, args ...any), timeout time.Duration, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		defer func() {
			if recovered := recover(); recovered != nil {
				if logger != nil {
					logger("panic serving %s %s: %v", r.Method, r.URL.Path, recovered)
				}
				Fail(w, http.StatusInternalServerError, "internal error")
			}
			if logger != nil {
				logger("%s %s %s", r.Method, r.URL.Path, time.Since(started).Round(time.Millisecond))
			}
		}()
		if timeout > 0 {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()
			r = r.WithContext(ctx)
		}
		next.ServeHTTP(w, r)
	})
}
