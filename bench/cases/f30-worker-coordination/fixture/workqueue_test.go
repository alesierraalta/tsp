package workqueue

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestManagerLifecycle(t *testing.T) {
	c := DefaultConfig()
	m, err := NewManager(c)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(100, 0)
	lease, ok := m.Start("a", "worker", now)
	if !ok {
		t.Fatal("start")
	}
	if _, ok := m.Start("a", "other", now); ok {
		t.Fatal("overlap")
	}
	if err := m.Finish(lease); err != nil {
		t.Fatal(err)
	}
	if m.Active() != 0 || m.Completions("a") != 1 {
		t.Fatal("counts")
	}
	if err := m.Finish(lease); err == nil {
		t.Fatal("duplicate")
	}
	if _, ok := m.Start("", "worker", now); ok {
		t.Fatal("key")
	}
	c.LeaseTTL = 0
	if _, err := NewManager(c); err == nil {
		t.Fatal("config")
	}
	c = DefaultConfig()
	c.Prefix = " "
	if c.Validate() == nil {
		t.Fatal("prefix")
	}
}

func TestLeaseBookGenerations(t *testing.T) {
	b := NewLeaseBook()
	now := time.Unix(100, 0)
	first, _ := b.Acquire("a", "one", now, time.Second)
	next, ok := b.Acquire("a", "two", now.Add(time.Second), time.Second)
	if !ok || next.Generation <= first.Generation {
		t.Fatal("renewal")
	}
	if b.Release("a", first.Generation) {
		t.Fatal("stale")
	}
	if !b.Release("a", next.Generation) || b.Count() != 0 {
		t.Fatal("release")
	}
	if _, ok := b.Current("a"); ok {
		t.Fatal("current")
	}
}

func TestCacheSnapshots(t *testing.T) {
	c := NewCache()
	_, generation, ok := c.Lookup("a")
	if ok {
		t.Fatal("empty")
	}
	if !c.Store("a", "one", generation) {
		t.Fatal("store")
	}
	if value, _, ok := c.Lookup("a"); !ok || value != "one" {
		t.Fatal("lookup")
	}
	if c.Invalidate("a") != 1 {
		t.Fatal("version")
	}
	if c.Store("a", "old", generation) {
		t.Fatal("stale store")
	}
	if c.Size() != 0 {
		t.Fatal("size")
	}
}

func TestFlightSharing(t *testing.T) {
	g := NewFlightGroup()
	started := make(chan struct{})
	release := make(chan struct{})
	f := g.Do("a", func() (string, error) { close(started); <-release; return "one", nil })
	<-started
	second := g.Do("a", func() (string, error) { return "two", nil })
	if f != second || g.Stats().Joined != 1 {
		t.Fatal("sharing")
	}
	close(release)
	if value, err := second.Await(context.Background()); err != nil || value != "one" {
		t.Fatal(value, err)
	}
	<-f.Done()
	if g.Stats().Active != 0 {
		t.Fatal("cleanup")
	}
}

func TestServiceAndBatch(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	s := NewService(func(key string) (string, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		if key == "missing" {
			return "", errors.New("missing")
		}
		return "value:" + key, nil
	})
	ctx := context.Background()
	first := s.FetchBatch(ctx, []string{"b", "a", "a"})
	if !first.OK() || len(first.Keys()) != 2 || first.Keys()[0] != "a" {
		t.Fatal(first)
	}
	if s.Cached() != 2 {
		t.Fatal("cache")
	}
	s.Fetch("a").Await(ctx)
	mu.Lock()
	n := calls
	mu.Unlock()
	if n != 2 {
		t.Fatal(n)
	}
	s.Invalidate("a")
	if value, err := s.Fetch("a").Await(ctx); err != nil || value != "value:a" {
		t.Fatal(value, err)
	}
	bad := s.FetchBatch(ctx, []string{"missing", ""})
	if bad.OK() || len(bad.Errors) != 2 {
		t.Fatal(bad)
	}
	if s.Stats().Started < 3 {
		t.Fatal("stats")
	}
}

func TestFutureCancellation(t *testing.T) {
	f := newFuture()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := f.Await(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	f.complete(Result{Value: "ready"})
	if value, err := f.Await(context.Background()); err != nil || value != "ready" {
		t.Fatal(value, err)
	}
}
