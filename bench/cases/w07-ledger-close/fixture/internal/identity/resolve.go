package identity

import "fmt"

// Resolver turns the subject a token asserts into the account it names. It is the only path from a
// token to an account, so the exactness of the comparison lives in one place.
type Resolver struct {
	directory *Directory
}

// NewResolver returns a resolver over a directory.
func NewResolver(directory *Directory) *Resolver {
	if directory == nil {
		directory = NewDirectory()
	}
	return &Resolver{directory: directory}
}

// Resolve returns the account a verified token's subject names. The subject is used as it arrived:
// it is not trimmed, folded or normalized first.
func (r *Resolver) Resolve(subject string) (Account, error) {
	if subject == "" || !ValidSubject(subject) {
		return Account{}, fmt.Errorf("%w: %q", ErrUnknownSubject, subject)
	}
	return r.directory.Find(subject)
}

// ResolveIn resolves a subject and proves the account belongs to the tenant the token claims. The
// tenant comparison is exact as well: two ids that differ in any byte are two tenants.
func (r *Resolver) ResolveIn(subject, tenant string) (Account, error) {
	account, err := r.Resolve(subject)
	if err != nil {
		return Account{}, err
	}
	if account.Tenant != tenant {
		return Account{}, fmt.Errorf("%w: subject %q is not in tenant %q", ErrTenantMismatch, subject, tenant)
	}
	return account, nil
}
