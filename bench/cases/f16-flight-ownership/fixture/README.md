# Flight ownership

In-process coordination helpers for asynchronous workers. Run `node --test`.

`LeasePool.acquire(key)` returns an ownership ticket, or `null` while the resource
is busy. `release(ticket)` returns whether that ticket released the resource.
Only the current owner may release a resource; a completed owner's ticket remains
inert for the lifetime of the pool. Keys are independent. Tickets are opaque and
must be passed back unchanged.

`FlightRegistry.run(key, operation)` returns a promise and coalesces callers onto
the current operation for that key. Both successful and failed operations free
their own registry entry. `invalidate(key)` detaches the registered operation
without cancelling its existing callers, allowing fresh work. Detached operations
cannot change ownership of the current entry. Operation callbacks run asynchronously.
