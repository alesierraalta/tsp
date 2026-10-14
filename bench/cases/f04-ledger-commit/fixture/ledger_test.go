package ledger

import (
	"errors"
	"testing"
)

func TestClaims(t *testing.T) {
	l := New(func(string, int) error { return nil })
	for index, want := range []bool{true, false} {
		got, err := l.Claim("invoice-1")
		if err != nil || got != want {
			t.Fatalf("claim %d: %v, %v", index, got, err)
		}
	}
}

func TestAdjustments(t *testing.T) {
	l := New(func(string, int) error { return nil })
	for _, delta := range []int{10, -3, 2} {
		if err := l.Add(delta); err != nil {
			t.Fatal(err)
		}
	}
	if got := l.Balance(); got != 9 {
		t.Fatalf("balance: %d", got)
	}
}

func TestAuditErrors(t *testing.T) {
	unavailable := errors.New("unavailable")
	l := New(func(string, int) error { return unavailable })
	if ok, err := l.Claim("invoice-1"); ok || !errors.Is(err, unavailable) {
		t.Fatalf("claim: %v, %v", ok, err)
	}
	if err := l.Add(10); !errors.Is(err, unavailable) {
		t.Fatal(err)
	}
	if l.Balance() != 0 {
		t.Fatal("unexpected balance")
	}
}
