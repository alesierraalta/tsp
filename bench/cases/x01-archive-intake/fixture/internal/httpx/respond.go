package httpx

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
)

// WithValue is a one-line alias for context.WithValue, so this package does not import context in
// every file that attaches a value to a request.
func WithValue(ctx context.Context, key, value any) context.Context {
	return context.WithValue(ctx, key, value)
}

// JSON writes one value as a JSON response. The content type is set before the status, so a client
// that streams the body knows what it got.
func JSON(w http.ResponseWriter, status int, value any) {
	body, err := json.Marshal(value)
	if err != nil {
		Fail(w, http.StatusInternalServerError, "cannot encode the response")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(status)
	_, _ = w.Write(append(body, '\n'))
}

// HTML writes one rendered page.
func HTML(w http.ResponseWriter, status int, page string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Length", strconv.Itoa(len(page)))
	w.WriteHeader(status)
	_, _ = io.WriteString(w, page)
}

// Bytes writes raw bytes with the content type given.
func Bytes(w http.ResponseWriter, status int, contentType string, body []byte) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
