package reaper

import (
	"reflect"
	"testing"
	"time"
)

func TestLifecycle(t *testing.T) {
	now := time.Date(2025, 1, 10, 0, 0, 0, 0, time.UTC)
	s := Store{Images: []Image{{"old", now.Add(-72 * time.Hour)}, {"fresh", now.Add(-time.Hour)}, {"boundary", now.Add(-24 * time.Hour)}}, Leases: []Lease{{[]string{"fresh"}}}}
	if !s.Referenced()["fresh"] {
		t.Fatal("reference")
	}
	if got := s.Reap(now, 24*time.Hour); !reflect.DeepEqual(got, []string{"old"}) {
		t.Fatal(got)
	}
	if len(s.Images) != 2 {
		t.Fatal(s.Images)
	}
	if got := s.Reap(now, 24*time.Hour); len(got) != 0 {
		t.Fatal(got)
	}
	if got := (&Store{}).Reap(now, 24*time.Hour); len(got) != 0 {
		t.Fatal(got)
	}
}
