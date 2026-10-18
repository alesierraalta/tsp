package issuer

import (
	"testing"
	"time"
)

func TestIssueAndLookup(t *testing.T) {
	now := time.Date(2025, 6, 15, 12, 30, 0, 0, time.UTC)
	next := int64(40)
	s := Issuer{Now: func() time.Time { return now }, Allocate: func() int64 { next++; return next }}
	for _, name := range []string{"alpha", "beta"} {
		c := s.Issue(name)
		if c.Subject != name || c.Serial != next || !c.Issued.Equal(now) || !c.Expires.Equal(time.Date(2026, 6, 15, 12, 30, 0, 0, time.UTC)) {
			t.Fatal(c)
		}
		if got, ok := s.Lookup(next); !ok || got != c {
			t.Fatal(got, ok)
		}
	}
	if _, ok := s.Lookup(999); ok {
		t.Fatal("unexpected certificate")
	}
	if len(s.Certificates) != 2 {
		t.Fatal(s.Certificates)
	}
}
