# Token pool

A sequential worker's reusable token budget. `New(capacity)` accepts a positive integer. `Reserve(n)` accepts a positive integer quantity if it fits in the available budget, deducts it and returns true. Rejected reservations leave the budget unchanged and return false. All available tokens may be reserved.

`Refund(n)` restores a nonnegative quantity, saturating at the configured capacity. This supports grants and returned reservations alike. Negative refunds panic. Callers use quantities whose sum fits in an int. `Available()` reports the remaining budget. The pool is intended for a single goroutine.

Run: `go test ./...`.
