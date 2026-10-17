package publisher

import (
	"reflect"
	"testing"
)

func TestPublication(t *testing.T) {
	store := NewMemoryStore()
	p := Publisher{Store: store}
	if got := p.Current(); got != nil {
		t.Fatalf("empty routes = %v", got)
	}
	for revision := uint64(1); revision <= 3; revision++ {
		routes := []string{"east", "west"}
		publish := p.Prepare(Plan{Revision: revision, Routes: routes, State: "retained"})
		routes[0] = "changed"
		publish()
		got := p.Current()
		if !reflect.DeepEqual(got, []string{"east", "west"}) {
			t.Fatalf("routes = %v", got)
		}
		got[0] = "changed"
		if p.Current()[0] != "east" {
			t.Fatal("routes alias storage")
		}
		if head, ok := store.Head(); !ok || head.Revision != revision {
			t.Fatalf("head = %+v, %v", head, ok)
		}
	}
}

func TestSweeping(t *testing.T) {
	s := NewMemoryStore()
	p := Publisher{Store: s}
	s.Stage(Plan{Revision: 1, State: "pending-deletion"})
	s.Stage(Plan{Revision: 2, State: "pending-deletion"})
	p.Prepare(Plan{Revision: 3, Routes: []string{"north"}, State: "retained"})()
	if n := p.Sweep(2); n != 2 {
		t.Fatalf("removed = %d", n)
	}
	if got := s.Revisions(); !reflect.DeepEqual(got, []uint64{3}) {
		t.Fatalf("revisions = %v", got)
	}
	if n := p.Sweep(2); n != 0 {
		t.Fatalf("second sweep removed = %d", n)
	}
	if got := p.Current(); !reflect.DeepEqual(got, []string{"north"}) {
		t.Fatalf("current = %v", got)
	}
}
