# Adapter semantics

An endpoint consumes the supplied iterator once. Iteration position belongs to the
caller; endpoints neither rewind iterators nor retain failed request fragments.
`beforeChunk` runs after the iterator has produced a chunk but before accepting it.
A `TransferError` may be retried according to its retryable flag. Other errors are
terminal. Each replacement request is a complete object, not an append operation.
The uploader owns construction of each request body.

Receipt storage's `beforeSave` represents the durability boundary. A rejected save
has no effect on receipt state. A committed receipt has a monotonically increasing
repository revision; a different checksum for an existing identifier is rejected.
The spool retains an entry until its receipt commits. Calling flush after a storage
failure must finish the outstanding delivery and receipt, without requiring the
application to enqueue that document again. Uploading the same bytes to the same
object key is safe because replacements are idempotent.

Only one flush runs per session. Entries queued during a flush are left for the next
flush snapshot. Transport history records completed upload operations, while endpoint
statistics include all requests and bytes accepted, including abandoned attempts.
Checksums are noncryptographic diagnostics, not authentication tokens.
