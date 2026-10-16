# Operations and reports

Dispatch request identifiers are scoped to an order's fulfillment cycle. Repeating
a request in that cycle returns the original receipt without consuming more units.
Changing the quantity on a repeated request is an error. A new cycle has its own
request namespace and receipts carry the cycle in which the goods were shipped.

Partial shipments keep the order open; exhausting its remaining units closes it.
Release is valid only for an open order. Reopen requires a closed order and enough
available stock. Rejected operations do not change inventory, orders or the journal.

Summary reports available stock, open orders, remaining reservations, receipt count
and journal length. Action totals sum quantities in the journal, not stock balances.
Order listings sort by id, while receipt listings have no ordering guarantee.
Receive and Reserve require positive quantities and nonempty identifiers. No disk
persistence or external transaction boundary is provided.

The command adapter (`go run ./cmd/stockdesk`) accepts colon-separated operations
as arguments: receive:sku:quantity, reserve:id:sku:quantity,
dispatch:id:request:quantity, release:id, reopen:id:quantity, balance:sku,
and order:id. It executes them on a fresh desk in argument order. Only balance
and order queries print output. Identifiers containing colons should use the Go
API instead of the command adapter.
