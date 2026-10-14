# Paged journal

An in-memory bounded journal for audit entries. Construct `Journal(capacity)` with a positive safe integer capacity. `append(entries)` atomically accepts a batch when the resulting entry count is at most capacity, returning true; otherwise it leaves the journal unchanged and returns false. Entries are kept in insertion order.

`page(offset, size)` accepts a nonnegative safe integer offset and positive safe integer size. It returns `{ items, next }`, with at most size entries. Pass the returned next offset into the next call to continue without gaps or repeats. next is null when there are no further entries. Read pagination only while the journal is not being modified.

Run: `node --test`.
