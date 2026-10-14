# Dispatch round

A sequential controller for sending jobs under a per-round quota. Construct with a positive limit using `NewRound(limit)` and call `Start()` to activate it.

`Dispatch(id)` accepts a job only in an active round, within its quota, and not already pending. Each accepted dispatch consumes one quota slot. `Acknowledge(id)` accepts only a pending job in an active round, removes it, and completes the round when no pending jobs remain. IDs identify individual deliveries; callers do not reuse a delivery id within a round or across rounds.

`Restart()` is permitted only after completion and starts an empty active round with its entire quota available again. `Cancel()` is permitted only while active and makes the controller permanently cancelled. No later event can change a cancelled controller. Invalid operations return false without changing state; successful operations return true. `State()` observes ready, active, completed, or cancelled. Calls are sequential, not concurrent.

Run: `go test ./...`.
