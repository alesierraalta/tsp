package auth

import "errors"

// ErrDenied is the refusal of an action a principal may not perform.
var ErrDenied = errors.New("auth: the principal may not perform this action")
