package auth

import (
	"fmt"
	"strings"
)

// Actions the HTTP surface checks. A route names one; the table below maps it to the scope a token
// must carry.
const (
	ActionRead      = "documents.read"
	ActionWrite     = "documents.write"
	ActionDelete    = "documents.delete"
	ActionExport    = "documents.export"
	ActionBatchRun  = "imports.run"
	ActionBatchView = "imports.read"
	ActionAdmin     = "admin.operate"
	ActionAudit     = "audit.read"
)

// scopesRequired is the one policy table. An action the table does not carry is refused, so a route
// added without a scope grants nothing.
var scopesRequired = map[string][]string{
	ActionRead:      {"documents:read"},
	ActionWrite:     {"documents:write"},
	ActionDelete:    {"documents:delete"},
	ActionExport:    {"documents:export", "documents:read"},
	ActionBatchRun:  {"imports:write"},
	ActionBatchView: {"imports:read"},
	ActionAdmin:     {"admin:operate"},
	ActionAudit:     {"audit:read"},
}

// Sinks are the actions that read or write the filesystem under the deployment root. They are kept
// together because a route that reaches one of them must check a scope first.
var Sinks = []string{ActionRead, ActionWrite, ActionDelete, ActionExport, ActionBatchRun, ActionAudit}

// ScopeAllowed reports whether a principal may perform an action, and which scope it lacked when it
// may not. Every scope the table names must be present; a token with one of two required scopes is
// refused, because the export reads documents and exports them.
func ScopeAllowed(p Principal, action string) (bool, string) {
	required, ok := scopesRequired[action]
	if !ok {
		return false, "action " + action + " has no policy"
	}
	for _, scope := range required {
		if !p.Has(scope) {
			return false, scope
		}
	}
	return true, ""
}

// Check refuses an action the principal may not perform. The returned error names the scope, which
// the audit trail records and the response does not.
func Check(p Principal, action string) error {
	allowed, missing := ScopeAllowed(p, action)
	if allowed {
		return nil
	}
	return fmt.Errorf("auth: %s requires %s", action, missing)
}

// InTenant reports whether a principal belongs to the tenant a record carries. The comparison is
// exact: two tenant ids that differ in any byte are two tenants.
func InTenant(p Principal, tenant string) bool { return p.Tenant == tenant }

// InAnyTenant reports whether the principal's tenant is one of the configured tenants. The
// configuration validates its list at startup, so this is a membership test, not a normalization.
func InAnyTenant(p Principal, tenants []string) bool {
	for _, tenant := range tenants {
		if tenant == p.Tenant {
			return true
		}
	}
	return false
}

// Actions lists the policy table as the operator page prints it: action and the scopes it needs.
func Actions() [][2]string {
	order := []string{ActionRead, ActionWrite, ActionDelete, ActionExport, ActionBatchRun, ActionBatchView, ActionAdmin, ActionAudit}
	out := make([][2]string, 0, len(order))
	for _, action := range order {
		out = append(out, [2]string{action, strings.Join(scopesRequired[action], " + ")})
	}
	return out
}

// KnownScope reports whether a scope is one the table names anywhere. It is used to refuse a token
// request that asks for a scope the service does not have.
func KnownScope(scope string) bool {
	for _, scopes := range scopesRequired {
		for _, known := range scopes {
			if known == scope {
				return true
			}
		}
	}
	return false
}
