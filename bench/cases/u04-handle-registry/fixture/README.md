# handle registry

Owns the handles members claim on the platform: one entry per handle, whatever case or Unicode
normalization the client sends.

- `new Registry({ locale })` creates a registry. `locale` is the dashboard locale the process runs in; it
  defaults to `en-US`. A `handles` Map may be passed to start from a persistent store.
- `claim(handle)` registers a handle and returns its entry. A handle must match `[\p{L}\p{M}\p{N}_.-]{3,20}` and
  may not be one of the reserved names (`admin`, `billing`, `support`, `café`), compared without regard to
  case. The registry holds one entry per handle, so a spelling that differs only in case or in Unicode
  normalization is the same handle and is refused with `ErrTaken`.
- `lookup(handle)` returns the entry, or `undefined`.
- `rename(current, next)` moves an entry to another handle under the same rules and refuses a handle that is
  already taken.
- `release(handle)` drops an entry and returns whether there was one.
- `list()` returns the entries sorted by handle, and `render(handle)` returns the `@name` form the page shows.

The suite runs with `node --test`.
