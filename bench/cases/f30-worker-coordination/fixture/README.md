# Worker coordination

This package supplies local coordination primitives for a multi-worker document
indexer: expiring job leases, shared asynchronous reads, versioned caching, and
batch collection. All public coordinator methods support concurrent callers.
It uses only the Go standard library. Run `go test ./...`.

`Manager.Start` reserves a namespaced key until its lease deadline. An active
reservation excludes any other acquisition, including by the same owner.
`Finish` records a completion and releases only the reservation represented by
its argument. Owners are worker labels, not unique reservation identities.
Rejected completions must leave coordination state unchanged.

`Service.Fetch` returns a future. Calls for the same resource within a cache
version share one backend operation. Successful values are cached; errors are
not. Batch reads deduplicate keys and retain per-key failures. Cancellation of
an await does not cancel a shared backend read or other subscribers.

See [consistency](docs/consistency.md) for expiry, invalidation, and backend
snapshot semantics. The package takes times explicitly so callers can use a
monotonic clock or deterministic simulation. No background timers are required.


The JSON-lines simulator (`go run ./cmd/queue`) accepts start, advance, finish,
leases, set, hold, fetch, invalidate, release, and values operations. It uses the
same library as workers. Names bind leases and futures; times advance in explicit
milliseconds. Only leases and values produce output. Hold/release controls backend
availability, set selects its next snapshot, and values awaits first/second futures.
