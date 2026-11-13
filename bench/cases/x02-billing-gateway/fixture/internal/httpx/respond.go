package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// contextWith stores the placeholder names on a request's context.
func contextWith(req *http.Request, names map[string]string) context.Context {
	return context.WithValue(req.Context(), paramsKey{}, names)
}

// Envelope is the body every JSON response carries. An error is never a bare string: a client reads
// the status for the class and the message for the reason.
type Envelope struct {
	OK    bool        `json:"ok"`
	Data  interface{} `json:"data,omitempty"`
	Error string      `json:"error,omitempty"`
}

// Write renders a successful response.
func Write(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(true)
	_ = encoder.Encode(Envelope{OK: true, Data: data})
}

// Fail renders a refusal. The message is what the client may see; the trail carries the detail.
func Fail(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(true)
	_ = encoder.Encode(Envelope{OK: false, Error: message})
}

// FailError renders a refusal and returns the status it used, for a handler that also records the
// reason in the trail.
func FailError(w http.ResponseWriter, err error) int {
	status := StatusOf(err)
	Fail(w, status, PublicMessage(err))
	return status
}

// StatusOf maps an error to the status a client reads.
func StatusOf(err error) int {
	switch {
	case err == nil:
		return http.StatusOK
	case errors.Is(err, ErrBadRequest):
		return http.StatusBadRequest
	case errors.Is(err, ErrUnauthorized):
		return http.StatusUnauthorized
	case errors.Is(err, ErrForbidden):
		return http.StatusForbidden
	case errors.Is(err, ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrConflict):
		return http.StatusConflict
	case errors.Is(err, ErrTooLarge):
		return http.StatusRequestEntityTooLarge
	case errors.Is(err, ErrUnprocessable):
		return http.StatusUnprocessableEntity
	case errors.Is(err, ErrRateLimited):
		return http.StatusTooManyRequests
	}
	return http.StatusInternalServerError
}

// PublicMessage returns the message a client may see for an error. An error the gateway did not classify
// is reported as a generic failure: the caller gets no internal detail, and the trail gets the detail.
func PublicMessage(err error) string {
	if err == nil {
		return ""
	}
	if StatusOf(err) == http.StatusInternalServerError {
		return "the request could not be completed"
	}
	return err.Error()
}

// Text renders a plain-text response, which is what the CSV export and the console page are.
func Text(w http.ResponseWriter, contentType, body string) {
	w.Header().Set("Content-Type", contentType+"; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, body)
}

// Attachment renders a download with the file name a client is offered.
func Attachment(w http.ResponseWriter, contentType, name, body string) {
	w.Header().Set("Content-Type", contentType+"; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, body)
}

// Decode reads a JSON body bounded by limit bytes. A body above the limit is refused before it is
// decoded, and a body with trailing content is refused: one request carries one object.
func Decode(req *http.Request, limit int64, target interface{}) error {
	if limit <= 0 {
		return fmt.Errorf("%w: the body cap is %d", ErrBadRequest, limit)
	}
	if req.Body == nil {
		return fmt.Errorf("%w: the request carries no body", ErrBadRequest)
	}
	reader := io.LimitReader(req.Body, limit+1)
	body, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrBadRequest, err)
	}
	if int64(len(body)) > limit {
		return fmt.Errorf("%w: the body is above %d bytes", ErrTooLarge, limit)
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return fmt.Errorf("%w: the request carries no body", ErrBadRequest)
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("%w: %v", ErrBadRequest, err)
	}
	if decoder.More() {
		return fmt.Errorf("%w: the body carries more than one object", ErrBadRequest)
	}
	return nil
}
