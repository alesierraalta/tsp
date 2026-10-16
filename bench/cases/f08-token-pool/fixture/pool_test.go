package pool

import "testing"

func TestWorkerReservations(t *testing.T) {
	p := New(10)
	if !p.Reserve(2) || !p.Reserve(3) {
		t.Fatal("reservation declined")
	}
	if p.Available() != 5 {
		t.Fatal("remaining budget")
	}
	p.Refund(2)
	if p.Available() != 7 {
		t.Fatal("returned budget")
	}
}

func TestDeclinedReservation(t *testing.T) {
	p := New(5)
	if p.Reserve(6) || p.Reserve(0) {
		t.Fatal("invalid reservation accepted")
	}
	if p.Available() != 5 {
		t.Fatal("budget changed")
	}
}

func TestReturnedReservation(t *testing.T) {
	p := New(5)
	p.Reserve(2)
	p.Refund(2)
	if p.Available() != 5 {
		t.Fatal("budget not restored")
	}
}
