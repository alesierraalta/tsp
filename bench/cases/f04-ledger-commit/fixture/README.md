# Ledger commit

`New(persist)` creates an empty in-memory ledger backed by an append-only audit callback.
The callback accepts a label and amount, is safe for simultaneous calls, and returns an error
when the audit entry cannot be stored. Callbacks must not re-enter the ledger.

`Claim(id)` records a string identifier and returns true only for its first successful claim.
Overlapping claims of the same identifier have exactly one winner. Audit entries may repeat;
consumers should use the returned boolean to decide whether to deliver the claimed benefit.

`Add(delta)` accepts signed integer adjustments. `Balance()` returns the sum of all successful
adjustments, including overlapping operations. All methods are safe for concurrent use.
A failed audit callback leaves ledger state unchanged and its error is returned to the caller.

Run `go test ./...`.
