package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"example.com/shiptrack/internal/auth"
	"example.com/shiptrack/internal/binder"
	"example.com/shiptrack/internal/egress"
	"example.com/shiptrack/internal/events"
	"example.com/shiptrack/internal/identity"
	"example.com/shiptrack/internal/labels"
	"example.com/shiptrack/internal/money"
	"example.com/shiptrack/internal/notify"
	"example.com/shiptrack/internal/statement"
	"example.com/shiptrack/internal/store"
	"example.com/shiptrack/internal/workspace"
)

// statusError is a refusal the service decided itself: it carries the status it answers with.
type statusError struct {
	status  int
	message string
}

func (e *statusError) Error() string { return e.message }

// Refuse returns an error that answers with a status and a message. A handler uses it for a refusal
// that no sentinel error describes.
func Refuse(status int, message string) error {
	return &statusError{status: status, message: message}
}

// JSON writes a value as JSON. A value that cannot be marshalled answers 500 with a fixed message: the
// error's text is a marshalling detail, and a detail is not what a client is told.
func JSON(w http.ResponseWriter, status int, value any) {
	body, err := json.Marshal(value)
	if err != nil {
		JSONError(w, http.StatusInternalServerError, "internal error", "response could not be encoded")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_, _ = w.Write(append(body, '\n'))
}

// JSONError writes the error envelope a client reads: a code and a message.
func JSONError(w http.ResponseWriter, status int, code, message string) {
	JSON(w, status, map[string]string{"error": code, "message": message})
}

// HTML writes a page. The body is markup this service rendered; it is never a value a request supplied.
func HTML(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

// File writes bytes as a download.
func File(w http.ResponseWriter, contentType, name string, body []byte) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// NoContent answers 204.
func NoContent(w http.ResponseWriter) { w.WriteHeader(http.StatusNoContent) }

// Error maps an error to a status and a body. A refusal the service knows answers with its own message;
// anything else answers 500 with a fixed message, so an internal detail — a path, a query text, a
// driver message — never reaches a client.
func Error(w http.ResponseWriter, err error) {
	if err == nil {
		JSONError(w, http.StatusInternalServerError, "internal error", "no error")
		return
	}
	var refused *statusError
	if errors.As(err, &refused) {
		JSONError(w, refused.status, http.StatusText(refused.status), refused.message)
		return
	}
	switch {
	case errors.Is(err, auth.ErrToken), errors.Is(err, auth.ErrExpired):
		JSONError(w, http.StatusUnauthorized, "unauthorized", "a valid token is required")
	case errors.Is(err, auth.ErrScope):
		JSONError(w, http.StatusForbidden, "forbidden", "the token does not grant this action")
	case errors.Is(err, auth.ErrAction):
		// A route wired with an action the policy lost fails closed, and the client is told the service
		// refused the action rather than that its token was fine.
		JSONError(w, http.StatusForbidden, "forbidden", "the requested action is not in the policy")
	case errors.Is(err, events.ErrTooLarge):
		JSONError(w, http.StatusRequestEntityTooLarge, "too large", "the request body is over the cap")
	case errors.Is(err, events.ErrInvalid), errors.Is(err, events.ErrDuplicate):
		JSONError(w, http.StatusBadRequest, "invalid", err.Error())
	case errors.Is(err, binder.ErrUnknownFilter), errors.Is(err, binder.ErrBadValue),
		errors.Is(err, binder.ErrLimit), errors.Is(err, binder.ErrUnknownOrder):
		JSONError(w, http.StatusBadRequest, "invalid selection", err.Error())
	case errors.Is(err, binder.ErrUnknownTable), errors.Is(err, binder.ErrUnknownColumn):
		JSONError(w, http.StatusInternalServerError, "internal error", "the report definition is not valid")
	case errors.Is(err, workspace.ErrNoObject), errors.Is(err, store.ErrNotFound):
		JSONError(w, http.StatusNotFound, "not found", "no such object")
	case errors.Is(err, workspace.ErrBadName), errors.Is(err, workspace.ErrOutside):
		JSONError(w, http.StatusBadRequest, "invalid name", "the name is not accepted")
	case errors.Is(err, statement.ErrPeriod), errors.Is(err, statement.ErrWindow):
		JSONError(w, http.StatusConflict, "period refused", err.Error())
	case errors.Is(err, statement.ErrTotals), errors.Is(err, statement.ErrTooMany):
		JSONError(w, http.StatusConflict, "statement refused", err.Error())
	case errors.Is(err, money.ErrInvalid), errors.Is(err, money.ErrOverflow), errors.Is(err, money.ErrAllocate):
		JSONError(w, http.StatusBadRequest, "invalid amount", err.Error())
	case errors.Is(err, identity.ErrInvalid), errors.Is(err, identity.ErrNotFound):
		JSONError(w, http.StatusBadRequest, "invalid identity", err.Error())
	case errors.Is(err, egress.ErrHost), errors.Is(err, egress.ErrShape), errors.Is(err, egress.ErrScheme):
		JSONError(w, http.StatusBadRequest, "endpoint refused", err.Error())
	case errors.Is(err, notify.ErrRefused), errors.Is(err, egress.ErrStatus), errors.Is(err, egress.ErrBody):
		JSONError(w, http.StatusBadGateway, "delivery failed", "the partner endpoint did not accept the notice")
	case errors.Is(err, labels.ErrRender), errors.Is(err, labels.ErrOutput):
		JSONError(w, http.StatusBadGateway, "render failed", "the renderer did not produce a label")
	case errors.Is(err, labels.ErrRunner), errors.Is(err, labels.ErrInput):
		JSONError(w, http.StatusInternalServerError, "render refused", "the renderer is not configured")
	default:
		JSONError(w, http.StatusInternalServerError, "internal error", "internal error")
	}
}

// BadRequest answers 400 with a fixed message for a malformed request.
func BadRequest(w http.ResponseWriter, message string) {
	JSONError(w, http.StatusBadRequest, "invalid request", message)
}

// Describe is the text an error is logged with: the handler's own refusals are readable, and anything
// else is passed through to the log, which is where an internal detail belongs.
func Describe(err error) string {
	if err == nil {
		return ""
	}
	var refused *statusError
	if errors.As(err, &refused) {
		return fmt.Sprintf("%d: %s", refused.status, refused.message)
	}
	return err.Error()
}
