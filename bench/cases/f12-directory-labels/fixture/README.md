# Directory labels

`findContact(contacts, query)` returns the first contact whose name matches the
query ignoring case and canonical Unicode representation, or null if absent.
Each contact has a string `name`; queries are strings.

`shortenLabel(label, limit)` returns at most `limit` Unicode code points from the
start of a string. It preserves their order and returns the whole label when it
fits. The limit must be a nonnegative integer; otherwise it throws RangeError.

Run the suite with `node --test`.
