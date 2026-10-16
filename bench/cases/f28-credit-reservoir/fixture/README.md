# Credit reservoir

A reservoir manages integer capacity and reservations. `New(capacity)` starts with
all units available; capacity must be nonnegative. `Reserve(units)` returns a
positive receipt, or zero without changing state if units are nonpositive or
unavailable. Each receipt represents the full requested number of units.

`Refund(receipt)` returns that reservation's units once. Unknown or already
settled receipts do nothing. Receipts are never reused.

`Resize(capacity)` preserves outstanding reservations. It returns false without
changing state if the requested capacity cannot accommodate all reserved units;
otherwise it returns true and makes the unreserved remainder available.
Available units are always nonnegative and never exceed capacity. Available plus
outstanding reserved units equals capacity after every accepted operation.
The reservoir is used serially, not concurrently.

Run `go test ./...`.
