package publish

import (
	"errors"
	"reflect"
	"testing"
)

func TestPublishOrder(t *testing.T) {
	var order []int
	steps := []Step{
		{Apply: func() error { order = append(order, 1); return nil }, Undo: func() error { return nil }},
		{Apply: func() error { order = append(order, 2); return nil }, Undo: func() error { return nil }},
		{Apply: func() error { order = append(order, 3); return nil }, Undo: func() error { return nil }},
	}
	if err := Publish(steps); err != nil || !reflect.DeepEqual(order, []int{1, 2, 3}) {
		t.Fatalf("order=%v err=%v", order, err)
	}
}

func TestPublishApplicationError(t *testing.T) {
	failure := errors.New("unavailable")
	if err := Publish([]Step{{Apply: func() error { return failure }, Undo: func() error { return nil }}}); !errors.Is(err, failure) {
		t.Fatalf("err=%v", err)
	}
}

func TestPublishEmpty(t *testing.T) {
	if err := Publish(nil); err != nil {
		t.Fatal(err)
	}
}
