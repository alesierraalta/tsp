// Package httpx is the HTTP plumbing the routes share: a segment router that captures path variables
// into the request context, the response writers, the middleware and the bounded request reader. The
// router matches whole segments only, so a route `/api/shipments/{id}` does not match
// `/api/shipments/a/b`, and a captured variable is read by name rather than by splitting the path again.
package httpx

import (
	"context"
	"net/http"
	"strings"
)

// contextKey is the type of the router's request keys, so no other package can collide with them.
type contextKey int

const (
	varsKey contextKey = iota
	requestKey
)

// segment is one part of a route pattern: a literal or a captured name.
type segment struct {
	literal string
	name    string
}

// route is one method and pattern.
type route struct {
	method   string
	segments []segment
	pattern  string
	handler  http.HandlerFunc
}

// Router matches requests by whole path segments.
type Router struct {
	routes []route
}

// NewRouter returns an empty router.
func NewRouter() *Router { return &Router{} }

// Handle registers a pattern. A pattern is slash-separated; a segment written as `{name}` captures the
// request's segment under that name. Anything else is a literal comparison.
func (r *Router) Handle(method, pattern string, handler http.HandlerFunc) {
	parts := strings.Split(strings.Trim(pattern, "/"), "/")
	segments := make([]segment, 0, len(parts))
	for _, part := range parts {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") && len(part) > 2 {
			segments = append(segments, segment{name: part[1 : len(part)-1]})
			continue
		}
		segments = append(segments, segment{literal: part})
	}
	r.routes = append(r.routes, route{method: method, segments: segments, pattern: pattern, handler: handler})
}

// ServeHTTP resolves the request to a route. A path that matches a pattern of another method answers
// 405; a path that matches nothing answers 404. The captured variables are stored in the request context
// before the handler runs.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	path := strings.Trim(req.URL.Path, "/")
	parts := []string{}
	if path != "" {
		parts = strings.Split(path, "/")
	}
	allowed := []string{}
	for _, candidate := range r.routes {
		vars, ok := candidate.match(parts)
		if !ok {
			continue
		}
		if candidate.method != req.Method {
			allowed = append(allowed, candidate.method)
			continue
		}
		req = req.WithContext(context.WithValue(req.Context(), varsKey, vars))
		candidate.handler(w, req)
		return
	}
	if len(allowed) > 0 {
		w.Header().Set("Allow", strings.Join(allowed, ", "))
		Error(w, &statusError{status: http.StatusMethodNotAllowed, message: "method not allowed"})
		return
	}
	Error(w, &statusError{status: http.StatusNotFound, message: "not found"})
}

// match compares the pattern's segments with the request's.
func (rt route) match(parts []string) (map[string]string, bool) {
	if len(parts) != len(rt.segments) {
		return nil, false
	}
	vars := map[string]string{}
	for i, want := range rt.segments {
		got := parts[i]
		if want.name != "" {
			if got == "" {
				return nil, false
			}
			vars[want.name] = got
			continue
		}
		if want.literal != got {
			return nil, false
		}
	}
	return vars, true
}

// Patterns lists the registered routes as `METHOD /pattern`, sorted by registration.
func (r *Router) Patterns() []string {
	patterns := make([]string, 0, len(r.routes))
	for _, rt := range r.routes {
		patterns = append(patterns, rt.method+" "+rt.pattern)
	}
	return patterns
}

// Path returns a captured path variable. A variable the pattern did not capture is "".
func Path(req *http.Request, name string) string {
	vars, ok := req.Context().Value(varsKey).(map[string]string)
	if !ok {
		return ""
	}
	return vars[name]
}

// WithRequestID records the request identifier a middleware generated.
func WithRequestID(req *http.Request, id string) *http.Request {
	return req.WithContext(context.WithValue(req.Context(), requestKey, id))
}

// RequestIDOf returns the request identifier a middleware generated, or "".
func RequestIDOf(req *http.Request) string {
	id, _ := req.Context().Value(requestKey).(string)
	return id
}
