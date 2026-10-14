# Request coordination

`SeatPool(capacity, authorize)` manages a non-negative integer number of seats.
`reserve(customer)` awaits the authorization callback, then returns whether a seat was assigned.
Authorization rejection propagates without consuming a seat. At most `capacity` reservations succeed,
even when requests overlap; `available` is the remaining seat count.

`ProfileCache(fetchProfile)` loads profiles by string key. `get(key)` returns the loaded value.
Overlapping requests for the same key share one load, and successful values remain cached.
Failed loads propagate and can be retried. Different keys load independently.

Run `node --test`.
