# Event channel

A synchronous in-memory event channel. Run `node --test`.

`new Channel({client, store, attempts})` accepts ordinary adapters: `client(value)` returns a success boolean; `store.read(after)` returns records with positive integer `sequence` strictly greater than `after`, in any order. The default client succeeds and the default store is empty. Adapters do not throw.

`enqueue(value)` accepts any value, including repeated values, as a separate submission. Each `flush()` makes one attempt for each pending submission. Successful submissions leave the queue; unsuccessful submissions leave after at most `attempts` total calls. The finite numeric attempts option is floored and clamped to at least one; default two. Empty flushes make no calls.

`refresh()` returns the store's new records. Each refresh reads only entries newer than the greatest sequence returned by any preceding refresh, starting after zero. An empty refresh preserves this position.
