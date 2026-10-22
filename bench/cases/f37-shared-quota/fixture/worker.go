package quota

import (
	"context"
	"errors"
)

// Job carries a tenant and an application-defined operation label.
type Job struct {
	Tenant    string
	Operation string
}

type Result struct {
	Job Job
	Err error
}

// Run acquires capacity for the duration of one synchronous callback.
// The caller must keep work within the configured lease duration.
func (l *Limiter) Run(ctx context.Context, job Job, work func(context.Context) error) error {
	if work == nil {
		return errors.New("work callback is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	slot, err := l.Acquire(job.Tenant)
	if err != nil {
		return err
	}
	defer l.Release(slot)
	return work(ctx)
}

// RunBatch preserves input order and reports errors per job.
// Jobs are executed serially; a failed job does not cancel later jobs.
func (l *Limiter) RunBatch(ctx context.Context, jobs []Job, work func(context.Context, Job) error) []Result {
	results := make([]Result, 0, len(jobs))
	for _, job := range jobs {
		item := job
		err := l.Run(ctx, item, func(ctx context.Context) error {
			if work == nil {
				return errors.New("work callback is required")
			}
			return work(ctx, item)
		})
		results = append(results, Result{Job: item, Err: err})
	}
	return results
}
