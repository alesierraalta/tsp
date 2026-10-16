# Leased store

An in-process value store with independently owned leases and subscriptions.
`put` replaces a value; `get` returns the current value or undefined. `remove`
removes the lookup without invalidating outstanding lease snapshots. `acquire`
returns a snapshot handle and `release` relinquishes ownership of that handle.
`subscribe` returns an opaque token; `unsubscribe` relinquishes that registration.
`close` clears lookup values without revoking outstanding handles or registrations.

Bookkeeping retains at most 32 relinquished handles of each kind in addition to
live handles, regardless of lookup lifetime. `retained()` reports all bookkeeping
entries, including idle ones. Payload results are independent of bookkeeping.

Run `node --test`. No dependencies are required.
