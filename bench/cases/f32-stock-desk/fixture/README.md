# Stock Desk

Stock Desk is a single-process reservation library for a small fulfillment desk.
It uses the standard library only; run `go test ./...`.

Receive increases available stock. Reserve moves units from available stock to an
open order. Dispatch consumes reserved units and produces an immutable receipt.
Release closes an open order and returns only its unshipped units to available
stock. Previously shipped goods are never restocked by releasing a reservation.

Closed orders may be reopened with a new quantity. Reopening is a fresh fulfillment
cycle, not a return of the previous shipment; it reserves new stock. All public
views are detached values. The desk is intended for serialized calls, not concurrent
use. See [docs/operations.md](docs/operations.md) for retry and reporting semantics.
