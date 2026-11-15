package auth

import (
	"fmt"
	"sort"
	"strings"
)

// ScopeDoc describes one scope the service knows: the string, what it lets a subject do, and the actions
// that require it. The console prints the table, and the test that checks the policy reads the same list.
type ScopeDoc struct {
	Scope   string
	Summary string
}

// Scopes lists every scope the policy table names, sorted, with its summary. A scope that is not in the
// table is not a scope the service has, whatever a token claims.
func Scopes() []ScopeDoc {
	return []ScopeDoc{
		{"admin:operate", "read the operator summary and sweep retention"},
		{"audit:read", "read the audit trail and export it"},
		{"documents:delete", "delete a committed object"},
		{"documents:export", "export the tenant's objects as CSV"},
		{"documents:read", "list and read committed objects"},
		{"documents:write", "reserved for the write path; the intake pipeline writes through imports:write"},
		{"imports:read", "read a staged batch, its report and its pages"},
		{"imports:write", "accept, resume and abort a batch"},
	}
}

// ScopeActions renders which actions one scope opens.
func ScopeActions(scope string) []string {
	var actions []string
	for action, scopes := range scopesRequired {
		for _, required := range scopes {
			if required == scope {
				actions = append(actions, action)
				break
			}
		}
	}
	sort.Strings(actions)
	return actions
}

// DescribeScopes renders the scope table as the page's rows: scope, what it does, and the actions.
func DescribeScopes() []string {
	lines := make([]string, 0, len(Scopes()))
	for _, doc := range Scopes() {
		lines = append(lines, fmt.Sprintf("%s: %s (actions: %s)", doc.Scope, doc.Summary, strings.Join(ScopeActions(doc.Scope), ", ")))
	}
	return lines
}

// Grantable reports whether a token request may ask for a scope: the scope must exist and the subject
// must be allowed to hold it. The archive grants every known scope to an authenticated subject; a
// deployment's identity provider decides who gets which.
func Grantable(scope string) error {
	if KnownScope(scope) {
		return nil
	}
	return fmt.Errorf("auth: scope %q is not one the archive knows", scope)
}

// Narrow returns the scopes of a request that the service will grant, and the names it refused.
func Narrow(scopes []string) (granted []string, refused []string) {
	seen := map[string]bool{}
	for _, scope := range scopes {
		trimmed := strings.TrimSpace(scope)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		if KnownScope(trimmed) {
			granted = append(granted, trimmed)
			continue
		}
		refused = append(refused, trimmed)
	}
	return granted, refused
}
