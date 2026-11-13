package access

import (
	"fmt"
	"sort"
	"sync"
)

// Account is one subject the directory carries. The directory is the authority on which tenant a
// subject belongs to: a token asserts a subject, and the directory says what it may see.
type Account struct {
	Subject string `json:"subject"`
	Tenant  string `json:"tenant"`
	Display string `json:"display"`
	Role    string `json:"role"`
}

// Directory is the account list of the service.
type Directory struct {
	mu       sync.RWMutex
	accounts []Account
}

// NewDirectory returns an empty directory.
func NewDirectory() *Directory { return &Directory{} }

// Add appends one account. A subject that already exists is refused: two accounts with one subject
// would make every lookup ambiguous, and an ambiguous identity is not an identity.
func (d *Directory) Add(a Account) error {
	if a.Subject == "" || a.Tenant == "" {
		return fmt.Errorf("access: an account needs a subject and a tenant")
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, existing := range d.accounts {
		if existing.Subject == a.Subject {
			return fmt.Errorf("access: subject %q already exists", a.Subject)
		}
	}
	d.accounts = append(d.accounts, a)
	return nil
}

// Accounts returns the accounts in subject order.
func (d *Directory) Accounts() []Account {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := append([]Account(nil), d.accounts...)
	sort.Slice(out, func(i, j int) bool { return out[i].Subject < out[j].Subject })
	return out
}

// Find returns the account of a subject. The comparison is exact: a caller that knows a subject
// knows that subject, and no other spelling of it.
func (d *Directory) Find(subject string) (Account, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	for _, a := range d.accounts {
		if a.Subject == subject {
			return a, true
		}
	}
	return Account{}, false
}

// InTenant reports whether the directory carries subject in tenant.
func (d *Directory) InTenant(subject, tenant string) bool {
	a, ok := d.Find(subject)
	return ok && a.Tenant == tenant
}
