# Shared gateway quota

A standard-library Go module for tenant admission across gateway instances. Gateways
in the same namespace use the same store and configuration. At most N live slots
may be held concurrently for a tenant across all instances, where N is the configured
limit. A slot not released within its lease is reclaimed. Releasing a slot is safe
at any time, including repeated completion notifications.

Construct a store with `store.New(nil)` for the system clock, then pass it to `New`
on each gateway. `Acquire` returns an opaque slot and `Release` completes it.
`Usage` reads occupancy; `Snapshot` reports local activity totals. Different tenants
have independent capacity. `Run` wraps a synchronous callback and `RunBatch` runs
ordered jobs with per-job outcomes. Work must stop when its slot's lease ends.

Run `go test ./...`. See [the service contract](docs/service.md) for configuration,
clock, and reporting semantics.
