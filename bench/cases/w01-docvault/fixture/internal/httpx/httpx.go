// Package httpx is the small HTTP layer docvault uses: a segment router, JSON and HTML responses,
// and the middleware every request passes through.
package httpx

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type paramsKey struct{}

// Params are the path parameters a route matched.
type Params map[string]string

// Param returns a matched path parameter, or "" when the route has none by that name.
func Param(r *http.Request, name string) string {
	params, _ := r.Context().Value(paramsKey{}).(Params)
	return params[name]
}

// WithParams returns a copy of the request carrying params. The router uses it; a test can too.
func WithParams(r *http.Request, params Params) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), paramsKey{}, params))
}

type route struct {
	method   string
	pattern  string
	segments []string
	handler  http.HandlerFunc
}

// Router matches method and path segments, with "{name}" segments captured as parameters.
type Router struct {
	routes []route
}

// NewRouter returns an empty router.
func NewRouter() *Router { return &Router{} }

// Handle registers a handler for a method and a path pattern such as "/api/documents/{id}".
func (rt *Router) Handle(method, pattern string, handler http.HandlerFunc) {
	if method == "" || !strings.HasPrefix(pattern, "/") {
		panic(fmt.Sprintf("httpx: %q %q is not a method and an absolute path", method, pattern))
	}
	rt.routes = append(rt.routes, route{
		method:   strings.ToUpper(method),
		pattern:  pattern,
		segments: segments(pattern),
		handler:  handler,
	})
}

// Get and Post are the two helpers the service registers routes with.
func (rt *Router) Get(pattern string, handler http.HandlerFunc) {
	rt.Handle(http.MethodGet, pattern, handler)
}
func (rt *Router) Post(pattern string, handler http.HandlerFunc) {
	rt.Handle(http.MethodPost, pattern, handler)
}

// ServeHTTP matches a request and calls the handler of the first route that fits. A path that no
// route matches is a 404, never a fallback that guesses.
func (rt *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	reqSegments := segments(r.URL.Path)
	for _, candidate := range rt.routes {
		if candidate.method != r.Method {
			continue
		}
		params, ok := match(candidate.segments, reqSegments)
		if !ok {
			continue
		}
		candidate.handler(w, WithParams(r, params))
		return
	}
	Fail(w, http.StatusNotFound, fmt.Sprintf("no route for %s %s", r.Method, r.URL.Path))
}

// Routes lists the registered patterns in registration order. The service publishes them on its
// health endpoint so an operator can see what a deployment actually serves.
func (rt *Router) Routes() []string {
	out := make([]string, 0, len(rt.routes))
	for _, r := range rt.routes {
		out = append(out, r.method+" "+r.pattern)
	}
	return out
}

func segments(p string) []string {
	trimmed := strings.Trim(p, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

func match(pattern, got []string) (Params, bool) {
	if len(pattern) != len(got) {
		return nil, false
	}
	params := make(Params, 2)
	for i, want := range pattern {
		if strings.HasPrefix(want, "{") && strings.HasSuffix(want, "}") {
			name := strings.TrimSuffix(strings.TrimPrefix(want, "{"), "}")
			if got[i] == "" {
				return nil, false
			}
			params[name] = got[i]
			continue
		}
		if want != got[i] {
			return nil, false
		}
	}
	return params, true
}

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

// WriteHTML writes an HTML response. The body is written as it is: every fragment the service
// serves comes from package htmlx, which escapes the values it interpolates.
func WriteHTML(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	w.Write([]byte(body))
}

// WriteText writes a plain-text response.
func WriteText(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	w.Write([]byte(body))
}

// Fail writes the standard error body.
func Fail(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, struct {
		Error string `json:"error"`
	}{Error: message})
}

// DecodeJSON reads a JSON request body of at most max bytes. A larger body is refused rather than
// truncated, because a truncated object decodes into something the caller did not send.
func DecodeJSON(w http.ResponseWriter, r *http.Request, max int64, into any) error {
	if r.Body == nil {
		return fmt.Errorf("empty body")
	}
	body := http.MaxBytesReader(w, r.Body, max)
	dec := json.NewDecoder(body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		return fmt.Errorf("decode body: %w", err)
	}
	return nil
}
