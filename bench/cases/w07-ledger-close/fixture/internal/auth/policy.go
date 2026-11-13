// Package auth is the token layer: the scopes a token carries, the actions the routes name, and the
// signature that makes a token ours.
package auth

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ErrForbidden is returned for a caller that does not hold what an action needs.
var ErrForbidden = errors.New("auth: forbidden")

// Actions are the operations the policy guards. Every route names one, and every handler asks the
// policy before it touches the store, the workspace or a partner, so an action missing from the table
// is refused rather than allowed.
const (
	ActionUsageWrite    = "usage:write"
	ActionUsageRead     = "usage:read"
	ActionInvoiceRead   = "invoices:read"
	ActionInvoiceExport = "invoices:export"
	ActionCloseRun      = "close:run"
	ActionDelivery      = "deliveries:write"
	ActionJournalRead   = "journal:read"
	ActionAdmin         = "admin:operate"
)

// ScopeSet is the set of scopes a token carries. Membership is exact: a scope is granted by the
// string it is, not by the string it starts with, so "invoices:readonly" grants nothing that
// "invoices:read" requires.
type ScopeSet map[string]bool

// NewScopeSet builds a set from a scope list. A blank entry is dropped rather than stored.
func NewScopeSet(scopes []string) ScopeSet {
	set := make(ScopeSet, len(scopes))
	for _, scope := range scopes {
		if scope = strings.TrimSpace(scope); scope != "" {
			set[scope] = true
		}
	}
	return set
}

// Has reports whether the set carries one scope exactly.
func (s ScopeSet) Has(scope string) bool { return s[scope] }

// List returns the scopes in sorted order.
func (s ScopeSet) List() []string {
	out := make([]string, 0, len(s))
	for scope := range s {
		out = append(out, scope)
	}
	sort.Strings(out)
	return out
}

// Principal is a verified caller: who, in which tenant, holding which scopes.
type Principal struct {
	Subject string
	Tenant  string
	Scopes  ScopeSet
}

// Policy decides which scopes an action requires. The table is fixed at construction; an action the
// table does not carry is refused.
type Policy struct {
	required map[string][]string
}

// NewPolicy returns the service policy.
func NewPolicy() *Policy {
	return &Policy{required: map[string][]string{
		ActionUsageWrite:    {"usage:write"},
		ActionUsageRead:     {"usage:read"},
		ActionInvoiceRead:   {"invoices:read"},
		ActionInvoiceExport: {"invoices:export", "invoices:read"},
		ActionCloseRun:      {"close:run"},
		ActionDelivery:      {"deliveries:write"},
		ActionJournalRead:   {"journal:read"},
		ActionAdmin:         {"admin:operate"},
	}}
}

// Required returns the scopes an action needs, or nil for an action the policy does not know.
func (p *Policy) Required(action string) []string { return p.required[action] }

// Allows reports whether the principal holds every scope the action requires, exactly. This is the
// only authorization decision the service makes.
func (p *Policy) Allows(principal Principal, action string) bool {
	needed, known := p.required[action]
	if !known || len(needed) == 0 {
		return false
	}
	for _, scope := range needed {
		if !principal.Scopes.Has(scope) {
			return false
		}
	}
	return true
}

// Check is Allows with an error a handler can return as it is.
func (p *Policy) Check(principal Principal, action string) error {
	if p.Allows(principal, action) {
		return nil
	}
	needed := p.Required(action)
	if len(needed) == 0 {
		return fmt.Errorf("%w: %q is not an action", ErrForbidden, action)
	}
	return fmt.Errorf("%w: %s needs %s, caller holds %s", ErrForbidden, action,
		strings.Join(needed, "+"), strings.Join(principal.Scopes.List(), ","))
}
