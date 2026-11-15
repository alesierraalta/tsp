package httpx

import "errors"

// The error classes a handler reports. Each maps to one status in StatusOf, and a handler that returns
// a classified error never leaks its text to a client it does not fit.
var (
	// ErrBadRequest is a request the gateway cannot read.
	ErrBadRequest = errors.New("bad request")
	// ErrUnauthorized is a request without a valid token.
	ErrUnauthorized = errors.New("unauthorized")
	// ErrForbidden is a request whose token does not carry the scope it needs.
	ErrForbidden = errors.New("forbidden")
	// ErrNotFound is a request for a record the tenant does not carry.
	ErrNotFound = errors.New("not found")
	// ErrConflict is a request that would replace a record that exists.
	ErrConflict = errors.New("conflict")
	// ErrTooLarge is a request above the deployment's body cap.
	ErrTooLarge = errors.New("request too large")
	// ErrUnprocessable is a request that is well formed and cannot be performed.
	ErrUnprocessable = errors.New("unprocessable")
	// ErrRateLimited is a request above the subject's allowance.
	ErrRateLimited = errors.New("rate limited")
)

// BadRequest wraps a reason as a bad request.
func BadRequest(reason string) error {
	return wrap(ErrBadRequest, reason)
}

// Unauthorized wraps a reason as an unauthorized request.
func Unauthorized(reason string) error {
	return wrap(ErrUnauthorized, reason)
}

// Forbidden wraps a reason as a forbidden request.
func Forbidden(reason string) error {
	return wrap(ErrForbidden, reason)
}

// NotFoundError wraps a reason as a not found.
func NotFoundError(reason string) error {
	return wrap(ErrNotFound, reason)
}

// Conflict wraps a reason as a conflict.
func Conflict(reason string) error {
	return wrap(ErrConflict, reason)
}

// Unprocessable wraps a reason as an unprocessable request.
func Unprocessable(reason string) error {
	return wrap(ErrUnprocessable, reason)
}

// RateLimited wraps a reason as a rate-limited request.
func RateLimited(reason string) error {
	return wrap(ErrRateLimited, reason)
}

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

// Error renders the reason, which an operator reads in the trail.
func (c *classified) Error() string { return c.class.Error() + ": " + c.reason }

// Unwrap returns the class, so errors.Is finds it.
func (c *classified) Unwrap() error { return c.class }
