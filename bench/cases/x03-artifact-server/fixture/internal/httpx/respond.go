package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// The error classes a handler reports.
var (
	// ErrBadRequest is a request the server cannot read.
	ErrBadRequest = errors.New("bad request")
	// ErrUnauthorized is a request without a valid token.
	ErrUnauthorized = errors.New("unauthorized")
	// ErrForbidden is a request whose token does not carry the scope it needs.
	ErrForbidden = errors.New("forbidden")
	// ErrNotFound is a request for a record the project does not carry.
	ErrNotFound = errors.New("not found")
	// ErrConflict is a request that would replace a record that exists.
	ErrConflict = errors.New("conflict")
	// ErrTooLarge is a request above a body cap.
	ErrTooLarge = errors.New("request too large")
	// ErrUnprocessable is a request that is well formed and cannot be performed.
	ErrUnprocessable = errors.New("unprocessable")
)

// Envelope is the body every JSON response carries.
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

// Fail renders a refusal. The message is what the client may see.
func Fail(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	encoder := json.NewEncoder(w)
	encoder.SetEscapeHTML(true)
	_ = encoder.Encode(Envelope{OK: false, Error: message})
}

// FailError renders a classified error and returns the status it used.
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
	}
	return http.StatusInternalServerError
}

// PublicMessage returns the message a client may see for an error.
func PublicMessage(err error) string {
	if err == nil {
		return ""
	}
	if StatusOf(err) == http.StatusInternalServerError {
		return "the request could not be completed"
	}
	return err.Error()
}

// Text renders a plain-text response.
func Text(w http.ResponseWriter, contentType, body string) {
	w.Header().Set("Content-Type", contentType+"; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, body)
}

// Attachment renders a download.
func Attachment(w http.ResponseWriter, contentType, name, body string) {
	w.Header().Set("Content-Type", contentType+"; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, body)
}

// Decode reads a JSON body bounded by limit bytes.
func Decode(req *http.Request, limit int64, target interface{}) error {
	if limit <= 0 {
		return fmt.Errorf("%w: the body cap is %d", ErrBadRequest, limit)
	}
	if req.Body == nil {
		return fmt.Errorf("%w: the request carries no body", ErrBadRequest)
	}
	body, err := io.ReadAll(io.LimitReader(req.Body, limit+1))
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

// BadRequest wraps a reason as a bad request.
func BadRequest(reason string) error { return wrap(ErrBadRequest, reason) }

// Unauthorized wraps a reason as an unauthorized request.
func Unauthorized(reason string) error { return wrap(ErrUnauthorized, reason) }

// Forbidden wraps a reason as a forbidden request.
func Forbidden(reason string) error { return wrap(ErrForbidden, reason) }

// NotFoundError wraps a reason as a not found.
func NotFoundError(reason string) error { return wrap(ErrNotFound, reason) }

// Conflict wraps a reason as a conflict.
func Conflict(reason string) error { return wrap(ErrConflict, reason) }

// Unprocessable wraps a reason as an unprocessable request.
func Unprocessable(reason string) error { return wrap(ErrUnprocessable, reason) }

// wrap returns a classified error carrying its reason.
func wrap(class error, reason string) error {
	if reason == "" {
		return class
	}
	return &classified{class: class, reason: reason}
}

// classified is an error class with a reason attached.
type classified struct {
	class  error
	reason string
}

// Error renders the reason.
func (c *classified) Error() string { return c.class.Error() + ": " + c.reason }

// Unwrap returns the class.
func (c *classified) Unwrap() error { return c.class }
