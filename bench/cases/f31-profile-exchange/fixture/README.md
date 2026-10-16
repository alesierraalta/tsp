# Profile Exchange

An in-process profile directory and transport adapter for directory integrations.
There are no runtime dependencies. Run `node --test` to exercise the project.

`Service` owns records and an audit stream. `service.client()` provides the same
request boundary a remote adapter would use. Records contain an immutable string
id and nullable text fields: name, email, team and note. Creation defaults omitted
fields to null; reads return detached copies. Duplicate ids and unknown fields
are rejected. Every successful mutation advances the revision and records an event.

Updates preserve fields not supplied by the caller. Supplying null explicitly
clears a field, and that distinction is preserved across the client transport.
The protocol and export contracts are specified in [docs/protocol.md](docs/protocol.md).

Queries support exact field matching, deterministic sorting, and offset pagination.
Removing a missing record is an error. The audit stream can be read by sequence or
record id; exported snapshots contain all records and their current revision.
