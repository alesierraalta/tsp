package quota_test

import (
	"context"
	"errors"
	q "sharedquota"
	"sharedquota/store"
	"sync"
	"testing"
	"time"
)

func gateway(t *testing.T, s *store.Store, limit int) *q.Limiter {
	t.Helper()
	l, err := q.New(q.Config{Namespace: "media", Limit: limit, Lease: time.Second}, s)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func TestAdmissionBursts(t *testing.T) {
	s := store.New(nil)
	a, b := gateway(t, s, 2), gateway(t, s, 2)
	for i := 0; i < 5; i++ {
		x, err := a.Acquire("alpha")
		if err != nil {
			t.Fatal(err)
		}
		y, err := b.Acquire("alpha")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := a.Acquire("alpha"); !errors.Is(err, q.ErrFull) {
			t.Fatal(err)
		}
		z, err := b.Acquire("beta")
		if err != nil {
			t.Fatal(err)
		}
		if n, err := a.Usage("alpha"); n != 2 || err != nil {
			t.Fatal(n, err)
		}
		b.Release(x)
		a.Release(y)
		b.Release(z)
		if n, _ := a.Usage("alpha"); n != 0 {
			t.Fatal(n)
		}
	}
	if a.Snapshot().Rejected != 5 {
		t.Fatal(a.Snapshot())
	}
}

func TestConfigurationAndIdentifiers(t *testing.T) {
	for _, c := range []q.Config{{}, {Namespace: "media", Limit: 1}, {Namespace: "media", Lease: time.Second}} {
		if _, err := q.New(c, store.New(nil)); err == nil {
			t.Fatal(c)
		}
	}
	if _, err := q.New(q.Config{Namespace: "media", Limit: 1, Lease: time.Second}, nil); err == nil {
		t.Fatal("missing store accepted")
	}
	l := gateway(t, store.New(nil), 1)
	for _, name := range []string{"", " ", "a\nb", "a\x00b"} {
		if _, err := l.Acquire(name); !errors.Is(err, q.ErrTenant) {
			t.Fatal(name, err)
		}
		if _, err := l.Usage(name); err == nil {
			t.Fatal(name)
		}
	}
	l.Release(q.Slot{})
}

func TestBatchAndCallbackOutcomes(t *testing.T) {
	l := gateway(t, store.New(nil), 1)
	failure := errors.New("conversion failed")
	jobs := []q.Job{{Tenant: "alpha", Operation: "convert"}, {Tenant: "alpha", Operation: "publish"}}
	results := l.RunBatch(context.Background(), jobs, func(_ context.Context, j q.Job) error {
		if j.Operation == "convert" {
			return failure
		}
		return nil
	})
	if len(results) != 2 || results[0].Err != failure || results[1].Err != nil || results[1].Job != jobs[1] {
		t.Fatal(results)
	}
	if n, _ := l.Usage("alpha"); n != 0 {
		t.Fatal(n)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := l.Run(ctx, jobs[0], func(context.Context) error { t.Fatal("callback called"); return nil }); err != context.Canceled {
		t.Fatal(err)
	}
	if err := l.Run(context.Background(), jobs[0], nil); err == nil {
		t.Fatal("missing callback accepted")
	}
}

func TestConcurrentIndependentTenants(t *testing.T) {
	l := gateway(t, store.New(nil), 8)
	var wg sync.WaitGroup
	for _, tenant := range []string{"alpha", "beta", "gamma", "delta"} {
		wg.Add(1)
		go func(tenant string) {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				err := l.Run(context.Background(), q.Job{Tenant: tenant}, func(context.Context) error { return nil })
				if err != nil {
					t.Error(err)
				}
			}
		}(tenant)
	}
	wg.Wait()
	if l.Snapshot().Accepted != 40 {
		t.Fatal(l.Snapshot())
	}
}
