package auth

import (
	"errors"
)

// Package auth issues and verifies the tokens the HTTP surface accepts, and holds the scope table
// that says which action a token must carry.

// Errors a verification returns. A handler maps all of them to 401 and never says which one, because
// the difference between an expired token and a forged one is not the client's business.
var (
	ErrMalformed = errors.New("auth: malformed token")
	ErrSignature = errors.New("auth: bad signature")
	ErrExpired   = errors.New("auth: token expired")
	ErrSubject   = errors.New("auth: token names no subject")
)
