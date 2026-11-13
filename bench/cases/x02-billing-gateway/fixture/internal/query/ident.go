package query

import "regexp"

// nameRe is the shape a table or column name has. A name that fails it is refused, because a name is
// the one part of a statement the engine cannot carry as a placeholder.
var nameRe = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// ValidName reports whether value is a name a statement may carry.
func ValidName(value string) bool {
	if len(value) > 63 {
		return false
	}
	return nameRe.MatchString(value)
}

// Identifier checks a name and returns the error a caller reports when it fails.
func Identifier(what, value string) (string, error) {
	if !ValidName(value) {
		return "", &NameError{What: what, Value: value}
	}
	return value, nil
}

// NameError is the refusal of a name that is not an identifier.
type NameError struct {
	What  string
	Value string
}

// Error renders the refusal.
func (e *NameError) Error() string {
	return "query: " + e.What + " " + quote(e.Value) + " is not an identifier"
}

// quote renders a value with its quotes, so an empty name is visible in the message.
func quote(value string) string {
	return `"` + value + `"`
}
