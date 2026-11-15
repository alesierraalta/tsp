// Package audit exposes the trail's queries: the entries a console lists, the sequence an operator can
// resume from, and the counts a status page prints. The trail is append-only, so every query here is a
// read in reverse order; none of them rewrites an entry.
package httpx

// This file is reserved for the trace helpers the API uses to describe a refused request. It carries no
// state: a trace is a string a log line and an audit entry share.

import (
	"fmt"
	"strings"
)

// Trace is the text an audit entry and a log line carry for one request: the method, the route and the
// request identifier. The query string is left out on purpose, because a filter value is the partner's.
func Trace(method, pattern, id string) string {
	parts := []string{method, pattern}
	if id != "" {
		parts = append(parts, "request="+id)
	}
	return strings.Join(parts, " ")
}

// Refusal describes a refusal for an audit entry: the status and the reason, with the reason bounded so
// a stored entry cannot grow with a message.
func Refusal(status int, err error) string {
	reason := ""
	if err != nil {
		reason = err.Error()
	}
	if len(reason) > 300 {
		reason = reason[:300]
	}
	return fmt.Sprintf("%d %s", status, reason)
}
