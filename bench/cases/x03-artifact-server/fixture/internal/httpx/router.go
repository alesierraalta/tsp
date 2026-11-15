// Package httpx is the artifact server's HTTP layer: the segment router and the response writers.
package httpx

import (
	"context"
	"net/http"
	"strings"
)

// Router is a small segment router.
type Router struct {
	routes   []route
	NotFound http.HandlerFunc
	Method   http.HandlerFunc
}

// route is one registered pattern.
type route struct {
	method   string
	pattern  string
	segments []string
	handler  http.HandlerFunc
}

// NewRouter returns a router whose unmatched requests are 404s.
func NewRouter() *Router {
	return &Router{NotFound: NotFound, Method: MethodNotAllowed}
}

// NotFound is the response an unmatched request gets.
func NotFound(w http.ResponseWriter, r *http.Request) {
	Fail(w, http.StatusNotFound, "no such endpoint")
}

// MethodNotAllowed is the response a matched path with an unmatched method gets.
func MethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	Fail(w, http.StatusMethodNotAllowed, "no such method for this endpoint")
}

// Handle registers one handler. A broken pattern is a programming error and panics.
func (r *Router) Handle(method, pattern string, handler http.HandlerFunc) {
	if !strings.HasPrefix(pattern, "/") {
		panic("httpx: pattern " + pattern + " does not start with a slash")
	}
	if handler == nil {
		panic("httpx: nil handler for " + pattern)
	}
	r.routes = append(r.routes, route{
		method: strings.ToUpper(method), pattern: pattern,
		segments: splitPath(pattern), handler: handler,
	})
}

// Get registers a GET handler.
func (r *Router) Get(pattern string, handler http.HandlerFunc) {
	r.Handle(http.MethodGet, pattern, handler)
}

// Post registers a POST handler.
func (r *Router) Post(pattern string, handler http.HandlerFunc) {
	r.Handle(http.MethodPost, pattern, handler)
}

// Delete registers a DELETE handler.
func (r *Router) Delete(pattern string, handler http.HandlerFunc) {
	r.Handle(http.MethodDelete, pattern, handler)
}

// ServeHTTP matches a request. A path registered under another method is a 405, not a 404.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	segments := splitPath(req.URL.Path)
	var matched bool
	for _, candidate := range r.routes {
		names, ok := match(candidate.segments, segments)
		if !ok {
			continue
		}
		matched = true
		if candidate.method != req.Method {
			continue
		}
		candidate.handler(w, WithParams(req, names))
		return
	}
	if matched && r.Method != nil {
		r.Method(w, req)
		return
	}
	if r.NotFound != nil {
		r.NotFound(w, req)
		return
	}
	http.NotFound(w, req)
}

// Patterns lists the registered routes.
func (r *Router) Patterns() []string {
	out := make([]string, 0, len(r.routes))
	for _, candidate := range r.routes {
		out = append(out, candidate.method+" "+candidate.pattern)
	}
	return out
}

// splitPath splits a path into its non-empty segments.
func splitPath(path string) []string {
	raw := strings.Split(strings.Trim(path, "/"), "/")
	out := make([]string, 0, len(raw))
	for _, segment := range raw {
		if segment != "" {
			out = append(out, segment)
		}
	}
	return out
}

// match reports whether a pattern matches a path, returning the placeholder names.
func match(pattern, segments []string) (map[string]string, bool) {
	if len(pattern) != len(segments) {
		return nil, false
	}
	names := make(map[string]string, 2)
	for i, expected := range pattern {
		if strings.HasPrefix(expected, "{") && strings.HasSuffix(expected, "}") {
			name := expected[1 : len(expected)-1]
			if name == "" || segments[i] == "" {
				return nil, false
			}
			names[name] = segments[i]
			continue
		}
		if expected != segments[i] {
			return nil, false
		}
	}
	return names, true
}

// paramsKey is the context key the placeholder names travel under.
type paramsKey struct{}

// WithParams returns a request carrying the placeholder names.
func WithParams(req *http.Request, names map[string]string) *http.Request {
	if len(names) == 0 {
		return req
	}
	return req.WithContext(context.WithValue(req.Context(), paramsKey{}, names))
}

// Param returns one placeholder of a matched request.
func Param(req *http.Request, name string) string {
	names, _ := req.Context().Value(paramsKey{}).(map[string]string)
	return names[name]
}
