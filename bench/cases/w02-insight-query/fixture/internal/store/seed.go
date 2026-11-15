package store

import "fmt"

// Seed loads the fixture data a fresh instance serves. It is a function rather than a file so a
// test can build the same store without touching the disk.
func (s *Store) Seed() error {
	accounts := []Account{
		{Subject: "kelvin", Tenant: "acme", Display: "Kelvin Ortiz", Role: "analyst"},
		{Subject: "amara", Tenant: "acme", Display: "Amara Silva", Role: "admin"},
		{Subject: "bob", Tenant: "globex", Display: "Bob Ceely", Role: "analyst"},
	}
	for _, a := range accounts {
		if err := s.AddAccount(a); err != nil {
			return err
		}
	}

	events := []Event{
		{ID: 1, Tenant: "acme", Account: "kelvin", Status: "booked", Amount: 1200, Created: "2024-04-01T09:00:00Z"},
		{ID: 2, Tenant: "acme", Account: "kelvin", Status: "booked", Amount: 800, Created: "2024-04-02T09:00:00Z"},
		{ID: 3, Tenant: "acme", Account: "kelvin", Status: "archived", Amount: 450, Created: "2024-04-03T09:00:00Z"},
		{ID: 4, Tenant: "acme", Account: "amara", Status: "booked", Amount: 5000, Created: "2024-04-04T09:00:00Z"},
		{ID: 5, Tenant: "globex", Account: "bob", Status: "booked", Amount: 9900, Created: "2024-04-05T09:00:00Z"},
		{ID: 6, Tenant: "globex", Account: "bob", Status: "archived", Amount: 100, Created: "2024-04-06T09:00:00Z"},
	}
	for _, e := range events {
		if err := s.AddEvent(e); err != nil {
			return err
		}
	}

	quotas := []Quota{
		{Tenant: "acme", Account: "kelvin", Limit: 5000, Used: 2000},
		{Tenant: "acme", Account: "amara", Limit: 20000, Used: 5000},
		{Tenant: "globex", Account: "bob", Limit: 15000, Used: 9900},
	}
	for _, q := range quotas {
		if err := s.AddQuota(q); err != nil {
			return err
		}
	}
	return nil
}

// TenantOf returns the tenant of one account subject, and whether the store knows the subject.
func (s *Store) TenantOf(subject string) (string, bool) {
	result, err := s.db.Query("SELECT tenant FROM accounts WHERE subject = ?", subject)
	if err != nil || len(result.Rows) == 0 {
		return "", false
	}
	tenant := result.Rows[0][0].String()
	if tenant == "" {
		return "", false
	}
	return tenant, true
}

// AccountCount reports how many accounts a tenant carries.
func (s *Store) AccountCount(tenant string) (int, error) {
	result, err := s.db.Query("SELECT subject FROM accounts WHERE tenant = ?", tenant)
	if err != nil {
		return 0, fmt.Errorf("store: count accounts of %q: %w", tenant, err)
	}
	return len(result.Rows), nil
}

// Accounts returns the account rows the store carries, in subject order. The directory the service
// authorizes against is built from them, so the store is the one place an account is declared.
func (s *Store) Accounts() ([]Account, error) {
	result, err := s.db.Query("SELECT subject, tenant, display, role FROM accounts ORDER BY subject")
	if err != nil {
		return nil, fmt.Errorf("store: accounts: %w", err)
	}
	out := make([]Account, 0, len(result.Rows))
	for _, values := range result.Rows {
		if len(values) != 4 {
			return nil, fmt.Errorf("store: an account row carries %d columns", len(values))
		}
		out = append(out, Account{
			Subject: values[0].String(),
			Tenant:  values[1].String(),
			Display: values[2].String(),
			Role:    values[3].String(),
		})
	}
	return out, nil
}
