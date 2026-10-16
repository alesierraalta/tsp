package dispatch

import "testing"

func TestDispatchRound(t *testing.T) {
	r := NewRound(2)
	if r.Dispatch("a") || !r.Start() || r.Start() {
		t.Fatal("start")
	}
	if !r.Dispatch("a") || r.Dispatch("a") || !r.Dispatch("b") || r.Dispatch("c") {
		t.Fatal("dispatch")
	}
	if r.Acknowledge("unknown") || !r.Acknowledge("a") || r.State() != "active" {
		t.Fatal("acknowledge")
	}
	if !r.Acknowledge("b") || r.State() != "completed" {
		t.Fatal("complete")
	}
}

func TestCancelRound(t *testing.T) {
	r := NewRound(1)
	r.Start()
	r.Dispatch("a")
	if !r.Cancel() || r.State() != "cancelled" || r.Dispatch("b") || r.Restart() {
		t.Fatal("cancel")
	}
}

func TestRestartRound(t *testing.T) {
	r := NewRound(2)
	r.Start()
	r.Dispatch("a")
	r.Acknowledge("a")
	if !r.Restart() || r.State() != "active" || !r.Dispatch("b") {
		t.Fatal("restart")
	}
}
