# Service contract

Namespace, limit, and lease must agree across gateways sharing a store. Namespace
and tenant identifiers are case-sensitive opaque strings; namespace is fixed for
the store's lifetime. Tenant identifiers must be nonblank and contain no control
line separators. A limit and lease must both be positive.

Store operations serialize across instances. The optional clock supplied to
`store.New` must be monotonic and concurrency-safe. A slot is live strictly before
its deadline; capacity is available at the deadline. There is no background worker:
reads and writes reclaim elapsed reservations. Slots may be completed through any
gateway connected to the same store, and must not be moved to another store.

`ErrFull` denotes exhausted capacity; invalid tenants are reported separately.
Neither outcome retains a reservation. A zero slot can be released harmlessly.
Local accepted and rejected counters count valid admission attempts; released
counts completion notifications for nonzero slots, not occupancy changes.

`RunBatch` does not retry jobs and preserves duplicates. Callback errors are
returned unchanged, and capacity is returned even if a callback panics. Cancellation
before admission does not call the callback. Cancellation after admission is passed
to the callback; the callback owns cancellation handling and lease compliance.
