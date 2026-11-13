package auth

import (
	"fmt"
	"sort"
	"strings"
)

// Actions are the operations the policy guards. Every route names one, and every handler asks the
// policy before it touches the vault, so an action missing from the table is refused rather than
// allowed.
const (
	ActionRead       = "documents:read"
	ActionWrite      = "documents:write"
	ActionDelete     = "documents:delete"
	ActionExport     = "documents:export"
	ActionAdmin      = "admin:operate"
	ActionAttachRead = "attachments:read"
	ActionAuditView  = "audit:read"
)

// ScopeSet is the set of scopes a token carries. Set membership is exact: a scope is granted by
// the string it is, not by the string it starts with, so "documents:readonly" grants nothing that
// "documents:read" requires.
type ScopeSet map[string]bool

// NewScopeSet builds a set from a scope list.
func NewScopeSet(scopes []string) ScopeSet {
	set := make(ScopeSet, len(scopes))
	for _, s := range scopes {
		if s = strings.TrimSpace(s); s != "" {
			set[s] = true
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

// Policy decides which scopes an action requires. The table is fixed at construction; a route the
// table does not carry has no scopes and is refused.
type Policy struct {
	required map[string][]string
}

// NewPolicy returns the service policy: reader scopes for the read actions, writer scopes for the
// mutating ones, and the operator scope for the administrative ones.
func NewPolicy() *Policy {
	return &Policy{required: map[string][]string{
		ActionRead:       {"documents:read"},
		ActionWrite:      {"documents:write"},
		ActionDelete:     {"documents:delete"},
		ActionExport:     {"documents:export", "documents:read"},
		ActionAdmin:      {"admin:operate"},
		ActionAttachRead: {"attachments:read"},
		ActionAuditView:  {"audit:read"},
	}}
}

// Required returns the scopes an action needs, or nil for an action the policy does not know.
func (p *Policy) Required(action string) []string {
	return p.required[action]
}

// Allows reports whether the principal holds every scope the action requires, exactly. This is the
// only authorization decision the service makes: handlers call it and nothing else compares scope
// strings.
func (p *Policy) Allows(pr Principal, action string) bool {
	need, ok := p.required[action]
	if !ok || len(need) == 0 {
		return false
	}
	for _, scope := range need {
		if !pr.Scopes.Has(scope) {
			return false
		}
	}
	return true
}

// Check is Allows with an error a handler can return as is.
func (p *Policy) Check(pr Principal, action string) error {
	if p.Allows(pr, action) {
		return nil
	}
	need := p.Required(action)
	if len(need) == 0 {
		return fmt.Errorf("%w: %q is not an action", ErrForbidden, action)
	}
	return fmt.Errorf("%w: %s needs %s, caller holds %s",
		ErrForbidden, action, strings.Join(need, "+"), strings.Join(pr.Scopes.List(), ","))
}

// InTenant reports whether the principal belongs to tenant. Tenants are compared exactly: two ids
// that differ in any byte are two tenants, and a comparison that folded case would let one tenant
// read another's documents whenever the ids happen to fold together.
func InTenant(pr Principal, tenant string) bool {
	return pr.Tenant == tenant
}
