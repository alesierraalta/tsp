// Package notify delivers the artifact server's events to the endpoints a project subscribed. A
// subscription names an endpoint, the events it wants and the secret its deliveries are signed with;
// an event is delivered only to the subscriptions that ask for it, and only after the outbound guard
// has allowed the endpoint.
package notify

import (
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// Errors a subscription reports.
var (
	// ErrInvalid is returned for a subscription that cannot be delivered to.
	ErrInvalid = errors.New("notify: invalid subscription")
	// ErrNotWanted is returned when a subscription does not ask for an event.
	ErrNotWanted = errors.New("notify: the subscription does not ask for this event")
)

// Subscription is one endpoint a project sends its events to.
type Subscription struct {
	ID       string   `json:"id"`
	Project  string   `json:"project"`
	Endpoint string   `json:"endpoint"`
	Secret   string   `json:"-"`
	Kinds    []string `json:"kinds"`
	Active   bool     `json:"active"`
}

// Validate checks the subscription. The endpoint is checked as a URL here, so a value that is not one
// is refused when the subscription is stored; whether the host may be called is the guard's decision,
// taken when a delivery runs.
func (s Subscription) Validate() error {
	if strings.TrimSpace(s.ID) == "" {
		return fmt.Errorf("%w: no id", ErrInvalid)
	}
	if strings.TrimSpace(s.Project) == "" {
		return fmt.Errorf("%w: %q names no project", ErrInvalid, s.ID)
	}
	if strings.TrimSpace(s.Endpoint) == "" {
		return fmt.Errorf("%w: %q carries no endpoint", ErrInvalid, s.ID)
	}
	parsed, err := url.Parse(s.Endpoint)
	if err != nil || parsed.Host == "" {
		return fmt.Errorf("%w: %q carries endpoint %q", ErrInvalid, s.ID, s.Endpoint)
	}
	if len(s.Secret) < 16 {
		return fmt.Errorf("%w: %q carries a %d byte secret", ErrInvalid, s.ID, len(s.Secret))
	}
	if len(s.Kinds) == 0 {
		return fmt.Errorf("%w: %q asks for no events", ErrInvalid, s.ID)
	}
	for _, kind := range s.Kinds {
		if !validKind(kind) {
			return fmt.Errorf("%w: %q asks for %q", ErrInvalid, s.ID, kind)
		}
	}
	return nil
}

// validKind reports whether an event kind is a lower-case dotted name.
func validKind(kind string) bool {
	if kind == "" || len(kind) > 64 {
		return false
	}
	for i := 0; i < len(kind); i++ {
		c := kind[i]
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '.', c == '-', c == '_':
		default:
			return false
		}
	}
	return true
}

// Wants reports whether the subscription asks for an event kind. A subscription that carries the
// wildcard asks for every kind.
func (s Subscription) Wants(kind string) bool {
	for _, wanted := range s.Kinds {
		if wanted == "*" || wanted == kind {
			return true
		}
	}
	return false
}

// KindList renders the kinds in order, for the audit record and the console.
func (s Subscription) KindList() string {
	kinds := append([]string(nil), s.Kinds...)
	sort.Strings(kinds)
	return strings.Join(kinds, ",")
}

// Event is one thing that happened in a project.
type Event struct {
	ID      string `json:"id"`
	Project string `json:"project"`
	Kind    string `json:"kind"`
	// Subject is the artifact or job the event is about.
	Subject string `json:"subject"`
	// Detail is one line an operator wrote, never a payload.
	Detail string `json:"detail,omitempty"`
	// At is when it happened, in RFC 3339.
	At string `json:"at"`
}

// Validate checks the event.
func (e Event) Validate() error {
	if e.ID == "" || e.Project == "" {
		return fmt.Errorf("%w: the event carries no id or no project", ErrInvalid)
	}
	if !validKind(e.Kind) {
		return fmt.Errorf("%w: the event carries kind %q", ErrInvalid, e.Kind)
	}
	if e.Subject == "" {
		return fmt.Errorf("%w: the event carries no subject", ErrInvalid)
	}
	return nil
}

// Matches reports whether a subscription and an event are one delivery: the projects agree, the
// subscription is active and it asks for the kind.
func Matches(sub Subscription, event Event) bool {
	return sub.Active && sub.Project == event.Project && sub.Wants(event.Kind)
}
