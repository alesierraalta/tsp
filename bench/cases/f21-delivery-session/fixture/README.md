# Delivery session

Two dependency-free helpers for remote I/O. Import `deliver` and `request` from
`src/index.js`.

`deliver(operation, payload, send, attempts = 3)` returns the successful response
or throws the last send error. The operation is a caller-assigned idempotency key:
every invocation of `send(key, payload)` for that delivery uses that same key and
payload. A remote service may commit a write before its acknowledgement is lost;
its key-based receipt store makes repeating the delivery safe. The sender owns
that receipt store, not this library. Separate deliveries with the same operation
are also allowed.

`request(budget, now, invoke, attempts = 3)` retries failed invocations and returns
the successful response. `now()` is a monotonic millisecond clock. `invoke` receives
the remaining milliseconds and must finish within that allowance. The budget
covers the whole request, including previous attempts, rather than each invocation.
When no allowance remains, the helper throws without invoking again. Attempts and
budgets are positive; errors are retryable. There is no sleeping or background work.

Run `node --test`.
