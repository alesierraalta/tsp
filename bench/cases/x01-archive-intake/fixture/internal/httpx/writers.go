package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// CSV writes a comma-separated body with the attachment disposition a spreadsheet expects.
func CSV(w http.ResponseWriter, filename string, body []byte) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=\""+SafeFilename(filename)+"\"")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// Fail writes the archive's one error shape: a JSON object with a message. A handler never writes a
// second body after a failure.
func Fail(w http.ResponseWriter, status int, message string) {
	if status < 400 {
		status = http.StatusInternalServerError
	}
	body, err := json.Marshal(map[string]string{"error": message})
	if err != nil {
		body = []byte(`{"error":"request failed"}`)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Length", strconv.Itoa(len(body)+1))
	w.WriteHeader(status)
	_, _ = w.Write(append(body, '\n'))
}

// ServerError is the response an unexpected failure gets: the detail goes to the log, never to the
// client.
func ServerError(w http.ResponseWriter, err error) {
	if err == nil {
		Fail(w, http.StatusInternalServerError, "internal error")
		return
	}
	if errors.Is(err, context.Canceled) {
		Fail(w, 499, "client closed the request")
		return
	}
	Fail(w, http.StatusInternalServerError, "internal error")
}

// SafeFilename renders a name for a Content-Disposition header without letting it carry the quote,
// the separator or the control character that would break the header.
func SafeFilename(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "" || out == "." || out == ".." {
		return "download"
	}
	return out
}

// DecodeJSON reads one JSON value from a bounded body. An unknown field is refused: a request that
// carries a field the service does not read is a client mistake, and guessing which fields to ignore
// has hidden real defects before.
func DecodeJSON(r *http.Request, limit int64, target any) error {
	if r.Body == nil {
		return fmt.Errorf("httpx: empty body")
	}
	if limit <= 0 {
		return fmt.Errorf("httpx: no body cap")
	}
	reader := http.MaxBytesReader(nil, r.Body, limit)
	dec := json.NewDecoder(reader)
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return fmt.Errorf("httpx: body is longer than %d bytes", limit)
		}
		return fmt.Errorf("httpx: decode: %w", err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("httpx: a second JSON value follows the body")
	}
	return nil
}

// ReadAll reads a whole body under a cap and refuses a longer one instead of reading its prefix.
func ReadAll(r io.Reader, limit int64) ([]byte, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("httpx: no read cap")
	}
	body, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, fmt.Errorf("httpx: read: %w", err)
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("httpx: body is longer than %d bytes", limit)
	}
	return body, nil
}
