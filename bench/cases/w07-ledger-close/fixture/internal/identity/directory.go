// Package identity is the account directory and the rule that resolves a subject to an account.
// Identity is exact: a subject is compared byte for byte against the subject the directory holds,
// because two strings a case-insensitive comparison calls equal are two different people when the
// difference is a Unicode case variant — "kelvin" and "\u212aelvin" fold together and are not the
// same subject.
package identity

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode"
)

// Errors the directory reports.
var (
	ErrUnknownSubject = errors.New("identity: unknown subject")
	ErrUnknownAccount = errors.New("identity: unknown account")
	ErrDuplicate      = errors.New("identity: duplicate key")
	ErrInvalid        = errors.New("identity: invalid account")
	ErrTenantMismatch = errors.New("identity: tenant mismatch")
)

// Currencies are the currencies the ledger bills in.
var Currencies = []string{"USD", "EUR", "GBP"}

// idRE is the account id rule. An account id is a path segment of a statement name, so it is
// lowercase kebab and nothing else.
var idRE = regexp.MustCompile(`^[a-z][a-z0-9-]{1,30}[a-z0-9]$`)

// Account is one billed account.
type Account struct {
	ID          string `json:"id"`
	Tenant      string `json:"tenant"`
	Subject     string `json:"subject"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
	Currency    string `json:"currency"`
	RateTable   string `json:"rate_table"`
}

// Validate refuses an account the ledger cannot bill: an id that is not a segment, a subject that
// carries a control character, an empty display name, an address without an "@", a currency the
// ledger does not bill, or a rate table id that is not one.
func (a Account) Validate() error {
	switch {
	case !idRE.MatchString(a.ID):
		return fmt.Errorf("%w: id %q", ErrInvalid, a.ID)
	case !idRE.MatchString(a.Tenant):
		return fmt.Errorf("%w: tenant %q", ErrInvalid, a.Tenant)
	case !ValidSubject(a.Subject):
		return fmt.Errorf("%w: subject %q", ErrInvalid, a.Subject)
	case !validText(a.DisplayName, 120):
		return fmt.Errorf("%w: display name %q", ErrInvalid, a.DisplayName)
	case !validEmail(a.Email):
		return fmt.Errorf("%w: email %q", ErrInvalid, a.Email)
	case !validCurrency(a.Currency):
		return fmt.Errorf("%w: currency %q", ErrInvalid, a.Currency)
	case !idRE.MatchString(a.RateTable):
		return fmt.Errorf("%w: rate table %q", ErrInvalid, a.RateTable)
	}
	return nil
}

// ValidSubject is the subject rule: 1 to 255 bytes of printable text, no leading or trailing space.
// A subject is otherwise opaque, and two subjects are equal when their bytes are.
func ValidSubject(subject string) bool {
	if subject == "" || len(subject) > 255 || strings.TrimSpace(subject) != subject {
		return false
	}
	return validText(subject, 255)
}

func validText(s string, limit int) bool {
	if s == "" || len(s) > limit || strings.TrimSpace(s) == "" {
		return false
	}
	for _, r := range s {
		if r == unicode.ReplacementChar || !unicode.IsPrint(r) {
			return false
		}
	}
	return true
}

// validEmail is a structural check, not a delivery check: one "@", text on both sides and a dotted
// domain, at most 254 bytes.
func validEmail(email string) bool {
	if len(email) > 254 || strings.Count(email, "@") != 1 {
		return false
	}
	local, domain, _ := strings.Cut(email, "@")
	if local == "" || !strings.Contains(domain, ".") ||
		strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") || strings.Contains(domain, "..") {
		return false
	}
	return validText(email, 254)
}

func validCurrency(code string) bool {
	for _, known := range Currencies {
		if known == code {
			return true
		}
	}
	return false
}

// Directory is the account directory of one deployment.
type Directory struct {
	mu        sync.RWMutex
	bySubject map[string]Account
	byID      map[string]Account
}

// NewDirectory returns an empty directory.
func NewDirectory() *Directory {
	return &Directory{bySubject: make(map[string]Account), byID: make(map[string]Account)}
}

// Add registers one account. A subject and an id each name exactly one account: a second account
// with the same subject is refused rather than replacing the first, because a replacement would
// silently move every token already issued.
func (d *Directory) Add(account Account) (Account, error) {
	if err := account.Validate(); err != nil {
		return Account{}, err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, exists := d.bySubject[account.Subject]; exists {
		return Account{}, fmt.Errorf("%w: subject %q", ErrDuplicate, account.Subject)
	}
	if _, exists := d.byID[account.ID]; exists {
		return Account{}, fmt.Errorf("%w: account %q", ErrDuplicate, account.ID)
	}
	d.bySubject[account.Subject] = account
	d.byID[account.ID] = account
	return account, nil
}

// Find returns the account a subject names, comparing the subject byte for byte. No folding,
// trimming or normalization is attempted, and the lookup is on the map key the subject is, not on a
// canonicalized form of it.
func (d *Directory) Find(subject string) (Account, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	account, ok := d.bySubject[subject]
	if !ok {
		return Account{}, fmt.Errorf("%w: %q", ErrUnknownSubject, subject)
	}
	return account, nil
}

// ByID returns the account with this id.
func (d *Directory) ByID(id string) (Account, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	account, ok := d.byID[id]
	if !ok {
		return Account{}, fmt.Errorf("%w: %q", ErrUnknownAccount, id)
	}
	return account, nil
}

// Tenant returns the accounts of one tenant, sorted by id.
func (d *Directory) Tenant(tenant string) []Account {
	d.mu.RLock()
	defer d.mu.RUnlock()
	accounts := make([]Account, 0, len(d.byID))
	for _, account := range d.byID {
		if account.Tenant == tenant {
			accounts = append(accounts, account)
		}
	}
	sort.Slice(accounts, func(i, j int) bool { return accounts[i].ID < accounts[j].ID })
	return accounts
}

// Len reports how many accounts the directory holds.
func (d *Directory) Len() int {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.byID)
}
