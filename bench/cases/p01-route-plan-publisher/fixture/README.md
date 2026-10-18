# Route plan publisher

A library for publishing revisioned route plans and reclaiming retired storage.
The current routes always come from the highest published revision. Sweeping
removes pending-deletion plans only; retained plans remain available.

`Prepare` stages a plan and returns its publication step. Revisions are unique,
positive, planner-assigned numbers. Routes and lifecycle states are supplied by
the planner. `Current` returns a copy of the published routes. `Sweep` returns
the number of plans removed through an inclusive revision watermark.

Storage is injected through `Store`. `MemoryStore` executes one atomic operation
at a time, allowing callers to choose the order of publication steps.

Run `go test ./...`.
