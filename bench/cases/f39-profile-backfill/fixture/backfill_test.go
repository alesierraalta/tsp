package backfill

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestDirectoryApply(t *testing.T) {
	repo := NewRepository([]Profile{
		{ID: "b", Handle: "Bea", Name: "Bea North", Team: "sales"},
		{ID: "a", Handle: "AL", Team: "sales"},
		{ID: "c", Handle: "Cam", Team: "support"},
	})
	index := NewIndex()
	worker := NewWorker(repo, NewRegistry(2), index)
	worker.Warm()
	report, err := worker.Run(context.Background(), Options{Team: "sales", Batch: 1})
	if err != nil || report.Applied != 1 || !reflect.DeepEqual(report.Selected, []string{"a"}) {
		t.Fatalf("first apply: %+v %v", report, err)
	}
	report, err = worker.Run(context.Background(), Options{Team: "sales"})
	if err != nil || report.Applied != 1 {
		t.Fatalf("next apply: %+v %v", report, err)
	}
	cards := index.Cards()
	if len(cards) != 2 || cards[0].Title != "AL" || !reflect.DeepEqual(cards[1].Terms, []string{"bea", "north"}) {
		t.Fatalf("cards: %+v", cards)
	}
	cards[0].Terms[0] = "changed"
	if index.Cards()[0].Terms[0] != "al" {
		t.Fatal("index snapshot aliases storage")
	}
	rows := repo.Snapshot()
	if rows[0].Attempts != 1 || rows[1].State != "complete" || rows[2].State != "pending" {
		t.Fatal(rows)
	}
	rows[0].State = "changed"
	if repo.Snapshot()[0].State != "complete" {
		t.Fatal("repository snapshot aliases storage")
	}
}

type failingSink struct{}

func (failingSink) Put(context.Context, Card) error { return errors.New("index unavailable") }

func TestApplyRetry(t *testing.T) {
	repo := NewRepository([]Profile{{ID: "a", Handle: "amy"}})
	registry := NewRegistry(1)
	worker := NewWorker(repo, registry, failingSink{})
	worker.Warm()
	report, err := worker.Run(context.Background(), Options{})
	if err != nil || len(report.Failures) != 1 || report.Applied != 0 {
		t.Fatalf("%+v %v", report, err)
	}
	worker.sink = NewIndex()
	report, err = worker.Run(context.Background(), Options{})
	if err != nil || report.Applied != 1 || repo.Snapshot()[0].Attempts != 2 {
		t.Fatalf("%+v %v", report, err)
	}
	if registry.Active() != 0 {
		t.Fatal("admission retained")
	}
}

func TestAdmissionAndValidation(t *testing.T) {
	registry := NewRegistry(1)
	c := registry.Open()
	ticket, ok := c.Acquire()
	if !ok {
		t.Fatal("admission refused")
	}
	w := NewWorker(NewRepository(nil), registry, NewIndex())
	w.Warm()
	if _, err := w.Run(context.Background(), Options{}); !errors.Is(err, ErrBusy) {
		t.Fatal(err)
	}
	c.Release(ticket)
	if _, err := w.Run(context.Background(), Options{}); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Run(context.Background(), Options{Batch: -1}); err == nil {
		t.Fatal("accepted negative batch")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := w.Run(ctx, Options{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
