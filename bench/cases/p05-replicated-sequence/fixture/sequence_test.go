package sequence

import (
	"reflect"
	"testing"
)

func TestSequence(t *testing.T) {
	store := &Store{}
	s := New(store)
	for i := 1; i <= 40; i++ {
		if got := s.Next(); got != i {
			t.Fatalf("got %d want %d", got, i)
		}
	}
	values := s.Values()
	values[0] = 999
	if s.Values()[0] != 1 {
		t.Fatal("aliased values")
	}
	s.Replicate()
	s.Promote()
	if len(s.Values()) != 40 || s.Next() != 41 {
		t.Fatal("promotion lost sequence")
	}
}
func TestEmptyRecovery(t *testing.T) {
	s := New(&Store{})
	s.Recover()
	if s.Next() != 1 {
		t.Fatal("first identifier")
	}
	s.Replicate()
	if !reflect.DeepEqual(s.Store.Primary, s.Store.Replica) {
		t.Fatal("replication")
	}
}
