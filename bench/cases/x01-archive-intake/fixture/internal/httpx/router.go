// Package httpx is the HTTP layer of the archive: a segment router, the response and error writers,
// the middleware around every handler, and the bounded readers a request body goes through.
package httpx

import (
	"net/http"
	"strings"
)

// Router is a small segment router. A pattern is a slash-separated path whose segments may be a
// literal or a placeholder in braces; a placeholder matches exactly one non-empty segment, so a
// placeholder can never swallow a slash and the handler always sees one name.
type Router struct {
	routes   []route
	NotFound http.HandlerFunc
}

// route is one registered pattern.
type route struct {
	method   string
	pattern  string
	segments []string
	handler  http.HandlerFunc
}

// NewRouter returns an empty router whose unmatched requests are 404s.
func NewRouter() *Router { return &Router{NotFound: DefaultNotFound} }

// DefaultNotFound is the response an unmatched request gets.
func DefaultNotFound(w http.ResponseWriter, r *http.Request) {
	Fail(w, http.StatusNotFound, "no such endpoint")
}

// Handle registers one handler. It panics on a pattern the router cannot read, because a route table
// is written once at startup and a broken pattern is a programming error, not a request error.
func (r *Router) Handle(method, pattern string, handler http.HandlerFunc) {
	if !strings.HasPrefix(pattern, "/") {
		panic("httpx: pattern " + pattern + " does not start with a slash")
	}
	if handler == nil {
		panic("httpx: nil handler for " + pattern)
	}
	r.routes = append(r.routes, route{
		method:   strings.ToUpper(method),
		pattern:  pattern,
		segments: splitPath(pattern),
		handler:  handler,
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

// Routes lists the registered patterns in registration order, as "METHOD /pattern".
func (r *Router) Routes() []string {
	out := make([]string, 0, len(r.routes))
	for _, rt := range r.routes {
		out = append(out, rt.method+" "+rt.pattern)
	}
	return out
}

// ServeHTTP matches the request's path and method. A path that matches a pattern under another
// method is a 405, not a 404, so a client learns that the endpoint exists.
func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	path := splitPath(req.URL.Path)
	methodMismatch := false
	for _, rt := range r.routes {
		params, ok := matchSegments(rt.segments, path)
		if !ok {
			continue
		}
		if rt.method != req.Method {
			methodMismatch = true
			continue
		}
		req = WithParams(req, params)
		rt.handler(w, req)
		return
	}
	if methodMismatch {
		w.Header().Set("Allow", r.allowFor(path))
		Fail(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if r.NotFound != nil {
		r.NotFound(w, req)
		return
	}
	DefaultNotFound(w, req)
}

// allowFor lists the methods the patterns matching path accept.
func (r *Router) allowFor(path []string) string {
	seen := map[string]bool{}
	var methods []string
	for _, rt := range r.routes {
		if _, ok := matchSegments(rt.segments, path); !ok {
			continue
		}
		if seen[rt.method] {
			continue
		}
		seen[rt.method] = true
		methods = append(methods, rt.method)
	}
	return strings.Join(methods, ", ")
}

// splitPath splits a path into its non-empty segments. A trailing slash is not a segment, so
// "/api/documents/" routes exactly like "/api/documents".
func splitPath(p string) []string {
	trimmed := strings.Trim(p, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

// matchSegments matches one pattern against one path. A placeholder takes one non-empty segment; a
// literal is compared byte for byte.
func matchSegments(pattern, path []string) (map[string]string, bool) {
	if len(pattern) != len(path) {
		return nil, false
	}
	params := map[string]string{}
	for i, want := range pattern {
		got := path[i]
		if strings.HasPrefix(want, "{") && strings.HasSuffix(want, "}") {
			name := strings.TrimSuffix(strings.TrimPrefix(want, "{"), "}")
			if name == "" || got == "" {
				return nil, false
			}
			params[name] = got
			continue
		}
		if want != got {
			return nil, false
		}
	}
	return params, true
}

// paramsKey is the request context key the matched parameters live under.
type paramsKey struct{}

// WithParams attaches the matched parameters to a request.
func WithParams(req *http.Request, params map[string]string) *http.Request {
	return req.WithContext(WithValue(req.Context(), paramsKey{}, params))
}

// Param returns one matched path parameter, or "" when the route did not carry it.
func Param(r *http.Request, name string) string {
	params, ok := r.Context().Value(paramsKey{}).(map[string]string)
	if !ok {
		return ""
	}
	return params[name]
}
