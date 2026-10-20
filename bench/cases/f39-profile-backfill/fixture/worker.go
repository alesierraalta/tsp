package backfill

import (
	"context"
	"sync"
)

type Worker struct {
	mu         sync.Mutex
	repository *Repository
	sink       Sink
	admission  admission
}

func NewWorker(r *Repository, registry *Registry, sink Sink) *Worker {
	return &Worker{repository: r, sink: sink, admission: admission{registry: registry}}
}

func (w *Worker) Warm() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.admission.warm()
}

func (w *Worker) Run(ctx context.Context, options Options) (Report, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	o, err := options.normalized()
	report := Report{Preview: o.Preview}
	if err != nil {
		return report, err
	}
	if err := ctx.Err(); err != nil {
		return report, err
	}
	ticket, allowed := w.admission.acquire()
	if !allowed {
		return report, ErrBusy
	}
	w.admission.warm()
	defer w.admission.release(ticket)
	var rows []Profile
	if o.Preview {
		rows = w.repository.Pending(o)
	} else {
		rows = w.repository.Claim(o)
	}
	for n, p := range rows {
		if err := ctx.Err(); err != nil {
			if !o.Preview {
				for _, unfinished := range rows[n:] {
					w.repository.finish(unfinished.ID, "pending")
				}
			}
			return report, err
		}
		card := BuildCard(p)
		report.record(p, card)
		if o.Preview {
			continue
		}
		if err := w.sink.Put(ctx, card); err != nil {
			report.failed(p.ID, err)
			w.repository.finish(p.ID, "pending")
			continue
		}
		w.repository.finish(p.ID, "complete")
		report.Applied++
	}
	return report, nil
}
