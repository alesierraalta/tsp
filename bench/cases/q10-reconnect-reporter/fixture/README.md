# Reconnect reporter

`Reporter({sink, pool, limit})` provides synchronous reporting and recovery work.
`sink.send(value)` returns a boolean; a false result is an ordinary failed attempt,
not an exception. `pool.acquire(value)` returns a unique non-null handle and
`pool.release(handle)` releases it. The default sink succeeds and the default pool
has no external effects. After any sequence of completed reports, at most one
attempt handle remains retained, available until the next report; a successful
report leaves none retained. `report(value)` returns the sink's boolean result.
Values are opaque, including negatives, empty strings and reserved property names.

`disconnect()` pauses recovery draining. `connect(id)` resumes it and schedules
recovery for that identifier. Pending recovery contains each identifier at most
once, ordered by its most recent connection, and retains at most `limit` entries
by dropping the oldest. `limit` is a finite number rounded down and clamped to
zero; non-finite values mean zero. `drain()` returns and clears pending identifiers
when connected, or returns an empty array without changing them when disconnected.
Identifiers are strings, including empty and reserved names. Repeated disconnects
are harmless. All dependencies are synchronous and no real I/O occurs.
