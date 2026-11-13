package auth

import (
	"errors"
	"fmt"
	"sort"
)

// ErrAction is reported for an action the policy table does not carry. An unknown action is refused,
// not treated as a public one: the table is the list of things a token can be allowed to do, and a
// route that asks for a name the table does not carry is a bug that fails closed.
var ErrAction = errors.New("auth: unknown action")

// Actions names what a route may require. The names are the routes' vocabulary, not the scopes':
// a handler asks for an action and the policy says which scopes that action needs.
const (
	ActionReadEvents     = "events:read"
	ActionWriteEvents    = "events:write"
	ActionReadShipments  = "shipments:read"
	ActionWriteShipments = "shipments:write"
	ActionReadStatements = "statements:read"
	ActionWriteClose     = "statements:close"
	ActionReadReports    = "reports:read"
	ActionExportReports  = "reports:export"
	ActionWriteDelivery  = "deliveries:write"
	ActionOperate        = "admin:operate"
	ActionReadAudit      = "audit:read"
)

// policy is the table: an action names the scopes a token must hold, all of them. A scope is a plain
// string and is compared exactly, so holding "statements:read" does not satisfy "statements:readonly"
// and holding "statements:readonly" does not satisfy "statements:read".
var policy = map[string][]string{
	ActionReadEvents:     {"events:read"},
	ActionWriteEvents:    {"events:write"},
	ActionReadShipments:  {"shipments:read"},
	ActionWriteShipments: {"shipments:write"},
	ActionReadStatements: {"statements:read"},
	ActionWriteClose:     {"statements:close", "statements:read"},
	ActionReadReports:    {"reports:read"},
	ActionExportReports:  {"reports:export", "reports:read"},
	ActionWriteDelivery:  {"deliveries:write"},
	ActionOperate:        {"admin:operate"},
	ActionReadAudit:      {"audit:read"},
}

// ScopesFor returns the scopes an action requires, sorted. The slice is a copy: a caller cannot edit
// the table through it.
func ScopesFor(action string) ([]string, error) {
	scopes, ok := policy[action]
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrAction, action)
	}
	copied := append([]string(nil), scopes...)
	sort.Strings(copied)
	return copied, nil
}

// Actions lists the actions the table carries, sorted.
func Actions() []string {
	actions := make([]string, 0, len(policy))
	for action := range policy {
		actions = append(actions, action)
	}
	sort.Strings(actions)
	return actions
}

// Allowed reports whether a token holding scopes may perform an action. An action the table does not
// carry is refused, and every scope the action names must be held: a close requires
// "statements:close" and "statements:read" together, which is why an export token cannot close a
// month.
func Allowed(scopes []string, action string) (bool, error) {
	required, err := ScopesFor(action)
	if err != nil {
		return false, err
	}
	for _, want := range required {
		if !holds(scopes, want) {
			return false, nil
		}
	}
	return true, nil
}

// holds reports whether a token carries a scope, exactly. It is a byte comparison: a scope is an
// opaque string the issuer wrote, and the service never lowercases or trims one to make it match.
func holds(scopes []string, want string) bool {
	for _, scope := range scopes {
		if scope == want {
			return true
		}
	}
	return false
}

// Granted lists the actions a token may perform, in table order.
func Granted(scopes []string) []string {
	actions := Actions()
	granted := make([]string, 0, len(actions))
	for _, action := range actions {
		if ok, err := Allowed(scopes, action); err == nil && ok {
			granted = append(granted, action)
		}
	}
	return granted
}

// Require is the check a handler makes: it returns ErrScope when the action is not granted, and
// ErrAction when the action is not in the table, so a route wired with a name the table lost fails
// closed and says why.
func Require(scopes []string, action string) error {
	ok, err := Allowed(scopes, action)
	if err != nil {
		return err
	}
	if !ok {
		required, _ := ScopesFor(action)
		return fmt.Errorf("%w: %s needs %v", ErrScope, action, required)
	}
	return nil
}
