package identity

import (
	"errors"
	"testing"
)

func account() Account {
	return Account{
		ID: "team-api", Tenant: "acme", Subject: "kelvin", DisplayName: "Team API",
		Email: "billing@example.com", Currency: "USD", RateTable: "standard",
	}
}

func TestAddRefusesASecondAccountWithTheSameKey(t *testing.T) {
	directory := NewDirectory()
	if _, err := directory.Add(account()); err != nil {
		t.Fatalf("add: %v", err)
	}
	second := account()
	second.ID = "team-web"
	if _, err := directory.Add(second); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("a repeated subject was accepted: %v", err)
	}
	third := account()
	third.Subject = "amara"
	if _, err := directory.Add(third); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("a repeated account id was accepted: %v", err)
	}
	if directory.Len() != 1 {
		t.Fatalf("the directory holds %d accounts", directory.Len())
	}
}

func TestFindComparesTheSubjectByteForByte(t *testing.T) {
	directory := NewDirectory()
	if _, err := directory.Add(account()); err != nil {
		t.Fatalf("add: %v", err)
	}
	if found, err := directory.Find("kelvin"); err != nil || found.ID != "team-api" {
		t.Fatalf("Find(kelvin) = %+v, %v", found, err)
	}
	// KELVIN SIGN plus "elvin" folds to "kelvin" and is a different subject.
	if _, err := directory.Find("\u212aelvin"); !errors.Is(err, ErrUnknownSubject) {
		t.Fatalf("a case-folded subject resolved: %v", err)
	}
	if _, err := directory.Find("Kelvin"); !errors.Is(err, ErrUnknownSubject) {
		t.Fatalf("a subject of another case resolved: %v", err)
	}
	if _, err := directory.Find("kelvin "); !errors.Is(err, ErrUnknownSubject) {
		t.Fatalf("a padded subject resolved: %v", err)
	}
}

func TestResolverChecksTheTenantExactly(t *testing.T) {
	directory := NewDirectory()
	if _, err := directory.Add(account()); err != nil {
		t.Fatalf("add: %v", err)
	}
	resolver := NewResolver(directory)
	if found, err := resolver.ResolveIn("kelvin", "acme"); err != nil || found.ID != "team-api" {
		t.Fatalf("ResolveIn(acme) = %+v, %v", found, err)
	}
	if _, err := resolver.ResolveIn("kelvin", "ACME"); !errors.Is(err, ErrTenantMismatch) {
		t.Fatalf("a tenant of another case resolved: %v", err)
	}
	if _, err := resolver.ResolveIn("kelvin", "globex"); !errors.Is(err, ErrTenantMismatch) {
		t.Fatalf("a subject of another tenant resolved: %v", err)
	}
	if _, err := resolver.Resolve(""); !errors.Is(err, ErrUnknownSubject) {
		t.Fatalf("an empty subject resolved: %v", err)
	}
}

func TestAccountValidationRefusesWhatItCannotBill(t *testing.T) {
	cases := map[string]func(*Account){
		"id with a space":    func(a *Account) { a.ID = "team api" },
		"uppercase tenant":   func(a *Account) { a.Tenant = "ACME" },
		"empty subject":      func(a *Account) { a.Subject = "" },
		"control in subject": func(a *Account) { a.Subject = "kel\nvin" },
		"padded subject":     func(a *Account) { a.Subject = " kelvin" },
		"email without @":    func(a *Account) { a.Email = "billing.example.com" },
		"email without dot":  func(a *Account) { a.Email = "billing@example" },
		"unknown currency":   func(a *Account) { a.Currency = "XXX" },
		"rate table path":    func(a *Account) { a.RateTable = "../standard" },
	}
	for name, mutate := range cases {
		broken := account()
		mutate(&broken)
		if err := broken.Validate(); err == nil {
			t.Fatalf("%s was accepted: %+v", name, broken)
		}
	}
}
