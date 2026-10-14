# Consistency boundaries

A reservation's generation is its fencing identity. Expiration permits a new
reservation, but does not authorize a former holder to complete the new work.
The owner label may be reused by a restarted worker. LeaseBook's Release checks
an exact generation; manager operations serialize inspection and release.
Expiry is inclusive: acquisition at the deadline is permitted.

Invalidation advances a resource's cache version synchronously. A Fetch invoked
after Invalidate returns must never subscribe to a pre-invalidation backend
snapshot. Reads already in progress may finish with the snapshot they started
with. Cache Store accepts only the generation captured at the start of the read.
Thus obsolete reads cannot repopulate a current cache. FlightGroup shares by
opaque caller-supplied identity and has no knowledge of cache versions.

Backends own their snapshot and error policy. They may block while accessing a
remote store. Future publication synchronizes results through the done channel;
subscribers can wait independently. Statistics count starts and joins, not cache
hits. A completed flight is removed so errors can be retried. Batch results sort
keys for presentation, without imposing backend execution order.
