package store

import (
	"fmt"
	"sort"
)

// PutAccount stores one account and refreshes the projection.
func (s *Store) PutAccount(account Account) error {
	if err := account.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureTenant(account.Tenant); err != nil {
		return err
	}
	if _, exists := s.accounts[account.ID]; exists {
		return fmt.Errorf("%w: account %q", ErrDuplicate, account.ID)
	}
	s.accounts[account.ID] = account
	return s.refresh()
}

// Account returns one account.
func (s *Store) Account(id string) (Account, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	account, ok := s.accounts[id]
	if !ok {
		return Account{}, fmt.Errorf("%w: account %q", ErrNotFound, id)
	}
	return account, nil
}

// Accounts returns the accounts of a tenant in id order.
func (s *Store) Accounts(tenant string) []Account {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Account, 0, len(s.accounts))
	for _, account := range s.accounts {
		if account.Tenant == tenant {
			out = append(out, account)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// AccountInTenant returns an account only when it belongs to the tenant given. A route that resolved
// an account id from a path calls this, so an id from another tenant is a not-found and never a
// cross-tenant read.
func (s *Store) AccountInTenant(tenant, id string) (Account, error) {
	account, err := s.Account(id)
	if err != nil {
		return Account{}, err
	}
	if account.Tenant != tenant {
		return Account{}, fmt.Errorf("%w: account %q in tenant %q", ErrNotFound, id, tenant)
	}
	return account, nil
}

// AccountByEmail returns the account of a tenant with the email given, compared byte for byte.
func (s *Store) AccountByEmail(tenant, email string) (Account, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, account := range s.accounts {
		if account.Tenant == tenant && account.Email == email {
			return account, nil
		}
	}
	return Account{}, fmt.Errorf("%w: account %q in tenant %q", ErrNotFound, email, tenant)
}

// EmailTaken reports whether a tenant already carries the email given. An address is unique inside a
// tenant, so a delivery that repeats one is refused instead of billing two accounts.
func (s *Store) EmailTaken(tenant, email string) bool {
	_, err := s.AccountByEmail(tenant, email)
	return err == nil
}
