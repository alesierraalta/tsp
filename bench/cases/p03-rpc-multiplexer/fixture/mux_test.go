package multiplexer

import (
	"reflect"
	"testing"
)

type wire struct{ frames []Frame }

func (w *wire) Send(f Frame) { w.frames = append(w.frames, f) }

func TestOrderedDelivery(t *testing.T) {
	w := &wire{}
	m := New(w)
	var got []string
	first := m.Submit(Read("a"), func(v string) { got = append(got, "a:"+v) })
	second := m.Submit(Read("b"), func(v string) { got = append(got, "b:"+v) })
	if first == second || len(w.frames) != 2 {
		t.Fatal("submission handles or frames")
	}
	m.Receive("one")
	m.Receive("two")
	m.Cancel(first)
	m.Receive("unsolicited")
	if !reflect.DeepEqual(got, []string{"a:one", "b:two"}) {
		t.Fatalf("responses = %v", got)
	}
}

func TestCancelledCallback(t *testing.T) {
	m := New(&wire{})
	called := false
	id := m.Submit(Read("a"), func(string) { called = true })
	m.Cancel(id)
	m.Cancel(id)
	m.Receive("one")
	if called {
		t.Fatal("cancelled callback called")
	}
}

func TestHandlers(t *testing.T) {
	for _, generation := range []int{1, 2} {
		data := &Data{Values: map[string]string{"a": "one"}, Replicas: map[string]int{}}
		if got := Handle(generation, Read("a"), data); got != "one" {
			t.Fatalf("read = %q", got)
		}
		if got := Handle(generation, Read("missing"), data); got != "" {
			t.Fatalf("missing = %q", got)
		}
	}
	data := &Data{Values: map[string]string{"a": "one"}, Replicas: map[string]int{}}
	if got := Handle(1, Replicate("a"), data); got != "one" || data.Replicas["a"] != 1 {
		t.Fatalf("replication = %q, %v", got, data.Replicas)
	}
	if got := Handle(2, Inspect("a"), data); got != "value=one replicas=1" || data.Replicas["a"] != 1 {
		t.Fatalf("inspection = %q, %v", got, data.Replicas)
	}
}
