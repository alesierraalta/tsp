# Operations

## Reading the trail

`GET /api/admin/audit` returns the entries of the caller's tenant, newest first. Every entry carries
a sequence number, the acting subject, the action, the object and the outcome (`ok`, `refused`,
`failed`). A refusal is recorded with the same weight as a success: an operator investigating an
incident reads what was attempted, not only what succeeded.

`POST /api/admin/purge` empties the trail of the caller's tenant and records that it did. The
record is the only entry the purge leaves behind.

## Reindexing

`POST /api/admin/reindex?prefix=documents` walks the object tree under the prefix and adds metadata
for objects the index does not carry. The walk goes through the vault, so a prefix outside the root
is refused before the walk starts, and an index is never rebuilt from a directory the service does
not serve.

The index is in memory. A restart starts from an empty index, so a deployment that keeps objects
across restarts runs the reindex once after the process comes up.

## Limits

The page size and the maximum upload are configuration, and the service refuses to start when they
are above the hard ceilings in `internal/config/limits.go`. Raising the ceilings is a code change on
purpose: a page of a hundred thousand rows and a one-gigabyte body are the two ways a document
service runs out of memory.

## Tenants

Tenant ids are configuration. A token naming a tenant the configuration does not carry is refused
with `403`, not mapped onto a default tenant: a request that names an unknown tenant has no
documents, and answering it with another tenant's set is the one mistake a metadata service cannot
make quietly.

## Content types

An object is served with the media type its extension names, and an extension the service does not
know is served as `application/octet-stream`. A stored `.html` object is therefore downloaded, never
rendered: the preview page is the only place document text reaches a browser as markup, and it
escapes it.
