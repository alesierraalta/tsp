package auth

import (
	"fmt"
	"sort"
)

// Action is one thing a route does, named so the policy table can say what it requires.
type Action string

// The actions the gateway's routes perform.
const (
	// ActionReadBilling reads plans, accounts and subscriptions.
	ActionReadBilling Action = "billing.read"
	// ActionWriteBilling changes a subscription.
	ActionWriteBilling Action = "billing.write"
	// ActionReadInvoices reads invoices.
	ActionReadInvoices Action = "invoices.read"
	// ActionWriteInvoices records a payment.
	ActionWriteInvoices Action = "invoices.write"
	// ActionReadReports runs a report.
	ActionReadReports Action = "reports.read"
	// ActionExportReport downloads a report.
	ActionExportReport Action = "reports.export"
	// ActionWriteUsage records usage.
	ActionWriteUsage Action = "usage.write"
	// ActionReadAudit reads the trail.
	ActionReadAudit Action = "audit.read"
	// ActionOperate runs an operator route.
	ActionOperate Action = "admin.operate"
)

// Rule is what one action requires.
type Rule struct {
	Action Action
	Scopes []string
	// Admin marks a rule that only a subject the deployment names as an operator may perform.
	Admin bool
}

// Policy is the table from an action to the scopes it requires. An action the table does not carry is
// refused: the table is the complete list of what the service can be asked to do, and a route missing
// from it is a route nobody granted.
type Policy struct {
	rules map[Action]Rule
}

// NewPolicy builds the table from its rules. A rule naming the same action twice is refused, because
// two requirements for one action means the looser of them would be enforced.
func NewPolicy(rules ...Rule) (*Policy, error) {
	policy := &Policy{rules: make(map[Action]Rule, len(rules))}
	for _, rule := range rules {
		if rule.Action == "" {
			return nil, fmt.Errorf("auth: a rule names no action")
		}
		if len(rule.Scopes) == 0 && !rule.Admin {
			return nil, fmt.Errorf("auth: action %q requires nothing", rule.Action)
		}
		for _, scope := range rule.Scopes {
			if !ValidScope(scope) {
				return nil, fmt.Errorf("auth: action %q requires %q, which is not a scope", rule.Action, scope)
			}
		}
		if _, exists := policy.rules[rule.Action]; exists {
			return nil, fmt.Errorf("auth: action %q is declared twice", rule.Action)
		}
		policy.rules[rule.Action] = Rule{
			Action: rule.Action, Scopes: append([]string(nil), rule.Scopes...), Admin: rule.Admin,
		}
	}
	return policy, nil
}

// DefaultPolicy is the policy a deployment starts with.
func DefaultPolicy() *Policy {
	policy, err := NewPolicy(
		Rule{Action: ActionReadBilling, Scopes: []string{ScopeBillingRead}},
		Rule{Action: ActionWriteBilling, Scopes: []string{ScopeBillingWrite}},
		Rule{Action: ActionReadInvoices, Scopes: []string{ScopeInvoicesRead}},
		Rule{Action: ActionWriteInvoices, Scopes: []string{ScopeInvoicesWrite}},
		Rule{Action: ActionReadReports, Scopes: []string{ScopeReportsRead}},
		Rule{Action: ActionExportReport, Scopes: []string{ScopeReportsExport, ScopeReportsRead}},
		Rule{Action: ActionWriteUsage, Scopes: []string{ScopeUsageWrite}},
		Rule{Action: ActionReadAudit, Scopes: []string{ScopeAuditRead}},
		Rule{Action: ActionOperate, Scopes: []string{ScopeAdminOperate}, Admin: true},
	)
	if err != nil {
		panic(err)
	}
	return policy
}

// Rule returns the rule of an action.
func (p *Policy) Rule(action Action) (Rule, error) {
	rule, ok := p.rules[action]
	if !ok {
		return Rule{}, fmt.Errorf("auth: action %q is not in the policy table", action)
	}
	return rule, nil
}

// Allows reports whether a principal may perform an action. A principal that is not the deployment's
// operator may not perform an action marked admin, whatever scopes its token carries.
func (p *Policy) Allows(principal Principal, action Action) error {
	rule, err := p.Rule(action)
	if err != nil {
		return err
	}
	if rule.Admin && !principal.Operator {
		return fmt.Errorf("%w: %q is an operator action", ErrDenied, action)
	}
	if !principal.Scopes.Requires(rule.Scopes...) {
		return fmt.Errorf("%w: %q requires %v", ErrDenied, action, rule.Scopes)
	}
	return nil
}

// Actions lists the actions the policy carries in order, for the console page.
func (p *Policy) Actions() []Action {
	actions := make([]string, 0, len(p.rules))
	for action := range p.rules {
		actions = append(actions, string(action))
	}
	sort.Strings(actions)
	out := make([]Action, 0, len(actions))
	for _, action := range actions {
		out = append(out, Action(action))
	}
	return out
}
