# Routing catalog

The catalog publishes complete routing snapshots and remembers the selected backend. An unsuccessful rebuild leaves the published snapshot available. Selection is durable when a persistent store is supplied.

`Catalog` provides `await rebuild()`, `choose(name)` and `route()`. Selection returns false for an unknown backend. Routes are detached copies of the selected backend's records. A rebuild returns true when it publishes and false when it cannot publish.

Constructor options:

- `backends`: backend names, default `['east', 'west']`.
- `store`: Map-compatible state store, default a new Map. Reuse the store to continue the catalog across instances with the same backend list.
- `lookup`: async function receiving a backend name and returning `{ok, records}`. `ok` reports whether the records are authoritative. Records have `name` and `address` fields. The default lookup provides one internal address per backend.

Run `node --test`.
