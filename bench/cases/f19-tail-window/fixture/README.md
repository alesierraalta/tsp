# Journal windows

An in-memory append-only journal. `append(entries)` adds values in order.
`window(start, end)` returns a detached half-open range without changing the journal.
Omitted bounds select the entire journal. Negative bounds are offsets from the
current end, and bounds outside the journal are clamped to its limits.
An empty or reversed interval returns an empty array.
