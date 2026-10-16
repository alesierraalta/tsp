package workqueue

import "context"

type Result struct {
	Value string
	Err   error
}

type Future struct {
	done   chan struct{}
	result Result
}

func newFuture() *Future {
	return &Future{done: make(chan struct{})}
}

func resolved(value string) *Future {
	f := newFuture()
	f.complete(Result{Value: value})
	return f
}

func (f *Future) complete(result Result) {
	f.result = result
	close(f.done)
}

func (f *Future) Await(ctx context.Context) (string, error) {
	select {
	case <-f.done:
		return f.result.Value, f.result.Err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (f *Future) Done() <-chan struct{} {
	return f.done
}
