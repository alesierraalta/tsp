package httpx

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// ErrBody is reported for a body the decoder refuses.
var ErrBody = errors.New("httpx: invalid body")

// ReadBody reads a request body under a bound. A declared Content-Length above the bound is refused
// before a byte is read; a body that lies about its length is refused after the read stops one byte
// over the bound, which is how a chunked body is caught as well.
func ReadBody(r *http.Request, limit int64) ([]byte, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("%w: bound %d", ErrBody, limit)
	}
	if r.ContentLength > limit {
		return nil, fmt.Errorf("%w: declared %d bytes, cap %d", ErrBody, r.ContentLength, limit)
	}
	if r.Body == nil {
		return nil, fmt.Errorf("%w: no body", ErrBody)
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBody, err)
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("%w: over %d bytes", ErrBody, limit)
	}
	return body, nil
}

// DecodeJSON reads one JSON object into dst, refusing an unknown field and a second value after the
// object. A body with a field the service does not carry is a request from a newer client, and silently
// dropping the field would make the client believe the service stored it.
func DecodeJSON(body []byte, dst any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("%w: %v", ErrBody, err)
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err == nil {
		return fmt.Errorf("%w: a second JSON value follows", ErrBody)
	} else if !errors.Is(err, io.EOF) {
		return fmt.Errorf("%w: %v", ErrBody, err)
	}
	return nil
}

// IntParam reads an integer query parameter. A parameter sent twice is refused, a value that does not
// parse is refused, and a value above max is refused rather than clamped: a caller that asked for a page
// of 1,000 rows is not silently handed 100.
func IntParam(r *http.Request, name string, fallback, min, max int) (int, error) {
	raw, err := Param(r, name)
	if err != nil {
		return 0, err
	}
	if raw == "" {
		return fallback, nil
	}
	if strings.TrimSpace(raw) != raw {
		return 0, fmt.Errorf("%w: %s carries surrounding space", ErrBody, name)
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%w: %s=%q is not an integer", ErrBody, name, raw)
	}
	if value < min || value > max {
		return 0, fmt.Errorf("%w: %s=%d is outside %d..%d", ErrBody, name, value, min, max)
	}
	return value, nil
}

// Param reads a single query parameter. A parameter sent more than once is refused: two values are not
// one value with two meanings, and choosing one is choosing for the caller.
func Param(r *http.Request, name string) (string, error) {
	values, ok := r.URL.Query()[name]
	if !ok {
		return "", nil
	}
	if len(values) != 1 {
		return "", fmt.Errorf("%w: %s carries %d values", ErrBody, name, len(values))
	}
	return values[0], nil
}

// BearerToken reads the Authorization header. The scheme is compared exactly and the header must be
// exactly two parts: `Bearer  abc` or `bearer abc` is not a header the service reads.
func BearerToken(r *http.Request) (string, error) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return "", fmt.Errorf("%w: no authorization header", ErrBody)
	}
	parts := strings.Split(header, " ")
	if len(parts) != 2 || parts[0] != "Bearer" || parts[1] == "" {
		return "", fmt.Errorf("%w: authorization header", ErrBody)
	}
	return parts[1], nil
}

// AcceptsJSON reports whether a request asked for JSON. The comparison is on the media type's prefix, so
// `application/json; charset=utf-8` is JSON and `text/html` is not; a request that sends no Accept header
// is answered with the page, which is what a browser expects.
func AcceptsJSON(r *http.Request) bool {
	return strings.HasPrefix(r.Header.Get("Accept"), "application/json")
}

// MethodNotAllowed answers 405 with the allowed methods.
func MethodNotAllowed(w http.ResponseWriter, allowed ...string) {
	w.Header().Set("Allow", strings.Join(allowed, ", "))
	JSONError(w, http.StatusMethodNotAllowed, "method not allowed", "the route does not answer this method")
}
