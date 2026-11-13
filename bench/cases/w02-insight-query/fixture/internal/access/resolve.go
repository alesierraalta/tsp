package access

import (
	"errors"
	"fmt"
	"strings"
)

// Resolver turns a verified token into the principal of a request. It is the one place the service
// decides which account a token acts as, so a handler never looks an account up itself.
type Resolver struct {
	dir *Directory
}

// NewResolver returns a resolver over a directory.
func NewResolver(dir *Directory) *Resolver { return &Resolver{dir: dir} }

// Resolve matches the subject a token asserts against the directory and returns the account it
// names. A subject the directory does not carry is refused, and a token whose tenant disagrees with
// the account's tenant is refused too: the token narrows what the directory allows, never widens it.
func (r *Resolver) Resolve(tok Token) (Principal, error) {
	if tok.Subject == "" {
		return Principal{}, fmt.Errorf("%w: empty subject", ErrUnknownSubject)
	}
	for _, account := range r.dir.Accounts() {
		if strings.EqualFold(account.Subject, tok.Subject) {
			if account.Tenant != tok.Tenant {
				return Principal{}, fmt.Errorf("%w: %q is not in tenant %q",
					ErrUnknownSubject, tok.Subject, tok.Tenant)
			}
			return Principal{
				Subject: account.Subject,
				Tenant:  account.Tenant,
				Role:    account.Role,
				Scopes:  NewScopes(tok.Scopes),
			}, nil
		}
	}
	return Principal{}, fmt.Errorf("%w: %q", ErrUnknownSubject, tok.Subject)
}

// StatusCode maps an access error to the HTTP status the service answers with.
func StatusCode(err error) int {
	switch {
	case err == nil:
		return 200
	case errors.Is(err, ErrForbidden):
		return 403
	default:
		return 401
	}
}
