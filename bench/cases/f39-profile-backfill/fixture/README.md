# Profile search backfill

This package rebuilds searchable profile cards for a directory. A worker accepts a
selection and either previews the planned cards or applies them to an index.
Preview changes nothing: profile status, attempts, and indexed cards remain intact.
At most the configured number of jobs run at once across all workers sharing a
registry, including the first job of a new worker. A busy worker returns ErrBusy;
callers may retry later. Completed profiles are omitted from subsequent applies.

Use NewRepository, NewRegistry and NewIndex to construct a Worker with NewWorker.
Run with a context and an Options value. The returned Report contains selected
identifiers and cards in identifier order. See docs/operations.md for selection,
error, and lifecycle semantics. Run the suite with `go test ./...`.
