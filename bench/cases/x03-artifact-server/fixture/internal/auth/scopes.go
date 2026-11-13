package auth

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// The scopes the artifact server grants.
const (
	// ScopeArtifactsRead reads artifacts and their records.
	ScopeArtifactsRead = "artifacts:read"
	// ScopeArtifactsWrite uploads, deletes and repacks artifacts.
	ScopeArtifactsWrite = "artifacts:write"
	// ScopeJobsRead reads jobs.
	ScopeJobsRead = "jobs:read"
	// ScopeJobsWrite starts and cancels jobs.
	ScopeJobsWrite = "jobs:write"
	// ScopeWebhooksRead reads subscriptions.
	ScopeWebhooksRead = "webhooks:read"
	// ScopeWebhooksWrite changes subscriptions and re-delivers events.
	ScopeWebhooksWrite = "webhooks:write"
	// ScopeAuditRead reads the trail.
	ScopeAuditRead = "audit:read"
	// ScopeAdminOperate runs the operator routes.
	ScopeAdminOperate = "admin:operate"
)

// scopeRe is the shape a scope has.
var scopeRe = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}:[a-z][a-z0-9_-]{0,31}$`)

// ValidScope reports whether value is a scope this service grants.
func ValidScope(value string) bool {
	return len(value) <= 64 && scopeRe.MatchString(value)
}

// Set is a set of scopes.
type Set map[string]bool

// ParseScopes builds a set, refusing a value that is not a scope.
func ParseScopes(values []string) (Set, error) {
	set := make(Set, len(values))
	for _, value := range values {
		scope := strings.TrimSpace(value)
		if scope == "" {
			continue
		}
		if !ValidScope(scope) {
			return nil, fmt.Errorf("auth: %q is not a scope", value)
		}
		set[scope] = true
	}
	return set, nil
}

// MustParseScopes is ParseScopes for a deployment's own table.
func MustParseScopes(values ...string) Set {
	set, err := ParseScopes(values)
	if err != nil {
		panic(err)
	}
	return set
}

// Has reports whether the set carries a scope.
func (s Set) Has(scope string) bool { return s[scope] }

// Requires reports whether the set carries every scope named.
func (s Set) Requires(scopes ...string) bool {
	for _, scope := range scopes {
		if !s.Has(scope) {
			return false
		}
	}
	return true
}

// Names returns the scopes in order.
func (s Set) Names() []string {
	names := make([]string, 0, len(s))
	for scope := range s {
		names = append(names, scope)
	}
	sort.Strings(names)
	return names
}

// Len returns the number of scopes.
func (s Set) Len() int { return len(s) }

// Rule is what one action requires.
type Rule struct {
	Scopes []string
	// Operator marks a rule only the deployment's operator subjects may perform.
	Operator bool
}

// Policy is the table from an action to the scopes it requires.
type Policy struct {
	rules map[string]Rule
}

// NewPolicy builds the table, refusing an action declared twice.
func NewPolicy(rules map[string]Rule) (*Policy, error) {
	policy := &Policy{rules: make(map[string]Rule, len(rules))}
	for action, rule := range rules {
		if action == "" {
			return nil, fmt.Errorf("auth: a rule names no action")
		}
		if len(rule.Scopes) == 0 && !rule.Operator {
			return nil, fmt.Errorf("auth: action %q requires nothing", action)
		}
		for _, scope := range rule.Scopes {
			if !ValidScope(scope) {
				return nil, fmt.Errorf("auth: action %q requires %q", action, scope)
			}
		}
		policy.rules[action] = Rule{Scopes: append([]string(nil), rule.Scopes...), Operator: rule.Operator}
	}
	return policy, nil
}

// DefaultPolicy is the table a deployment starts with.
func DefaultPolicy() *Policy {
	policy, err := NewPolicy(map[string]Rule{
		"artifacts.read":  {Scopes: []string{ScopeArtifactsRead}},
		"artifacts.write": {Scopes: []string{ScopeArtifactsWrite}},
		"jobs.read":       {Scopes: []string{ScopeJobsRead}},
		"jobs.write":      {Scopes: []string{ScopeJobsWrite}},
		"webhooks.read":   {Scopes: []string{ScopeWebhooksRead}},
		"webhooks.write":  {Scopes: []string{ScopeWebhooksWrite}},
		"audit.read":      {Scopes: []string{ScopeAuditRead}},
		"admin.operate":   {Scopes: []string{ScopeAdminOperate}, Operator: true},
	})
	if err != nil {
		panic(err)
	}
	return policy
}

// Allows reports whether a principal may perform an action.
func (p *Policy) Allows(principal Principal, action string) error {
	rule, ok := p.rules[action]
	if !ok {
		return fmt.Errorf("%w: action %q is not in the policy table", ErrDenied, action)
	}
	if rule.Operator && !principal.Operator {
		return fmt.Errorf("%w: %q is an operator action", ErrDenied, action)
	}
	if !principal.Scopes.Requires(rule.Scopes...) {
		return fmt.Errorf("%w: %q requires %v", ErrDenied, action, rule.Scopes)
	}
	return nil
}

// Principal is the caller a request acts as.
type Principal struct {
	Subject  string
	Project  string
	Scopes   Set
	Operator bool
}

// NewPrincipal builds a principal from verified claims.
func NewPrincipal(claims Claims, operator bool) (Principal, error) {
	scopes, err := ParseScopes(claims.Scopes)
	if err != nil {
		return Principal{}, err
	}
	principal := Principal{Subject: claims.Subject, Project: claims.Project, Scopes: scopes, Operator: operator}
	if principal.Subject == "" || principal.Project == "" {
		return Principal{}, fmt.Errorf("%w: the principal is incomplete", ErrDenied)
	}
	return principal, nil
}

// Anonymous returns the principal a request without a token acts as.
func Anonymous() Principal { return Principal{} }

// IsAnonymous reports whether the principal carries no subject.
func (p Principal) IsAnonymous() bool { return p.Subject == "" }

// In reports whether the principal belongs to a project.
func (p Principal) In(project string) bool {
	return !p.IsAnonymous() && p.Project == project
}

// May reports whether the principal may perform an action.
func (p Principal) May(policy *Policy, action string) error {
	if p.IsAnonymous() {
		return fmt.Errorf("%w: the request carries no token", ErrDenied)
	}
	return policy.Allows(p, action)
}
