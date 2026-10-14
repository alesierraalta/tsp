# Projection store

`ProjectionStore(project)` keeps keyed rows. `update(key, value)` replaces a row.
`page()` returns projected values in insertion order, using one cached projection
for the current state. Returned arrays are independent; projectors are pure.
Only the current state's projection may remain retained. `retainedPages` reports
the number of cached projections, not the number of rows.

`mergeBatches(batches, keyOf)` returns one row per key in first-key order; the last
row for a key wins. The key callback is evaluated exactly once per incoming row,
and merging requires linear work in the number of incoming rows. Inputs are not
mutated. Callbacks may be instrumented without changing their return values.

Run `node --test`.
