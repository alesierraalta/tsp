# Member scheduler

`Scheduler({members, client, store})` manages a member set and staged work.
`members` is an iterable of member identifiers; duplicates collapse. `remove(id)`
removes that member, with unknown identifiers ignored. `inspect()` calls the
synchronous `client.check(id)` for each remaining member and returns the current
members; `store` is a Map-compatible destination for the `members` snapshot.
The default client returns `ok`. A check returns `ok`, `timeout`, or `failed`:
only confirmed failure removes a member, while a timeout preserves membership.

`stage(items)` appends an array of work values, preserving order and duplicates.
`promote()` moves all staged work to the ready queue. `tick()` returns and removes
all ready work in order.
Empty operations return empty arrays where applicable. Identifiers and work values
are opaque, including empty strings and reserved property names. No real I/O occurs.
