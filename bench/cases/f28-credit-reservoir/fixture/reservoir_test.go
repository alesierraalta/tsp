package reservoir

import "testing"

func TestReservations(t *testing.T) {
	r := New(5)
	id := r.Reserve(2)
	if id == 0 || r.Available() != 3 {
		t.Fatal("reservation")
	}
	if r.Reserve(4) != 0 {
		t.Fatal("unavailable units")
	}
	r.Refund(id)
	if r.Available() != 5 {
		t.Fatal("settlement")
	}
}

func TestCapacityChanges(t *testing.T) {
	r := New(3)
	if r.Reserve(1) == 0 {
		t.Fatal("reservation")
	}
	if !r.Resize(5) || r.Available() != 4 {
		t.Fatal("capacity")
	}
	if r.Resize(0) || r.Capacity() != 5 {
		t.Fatal("rejected capacity")
	}
}
