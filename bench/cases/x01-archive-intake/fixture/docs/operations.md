# Operations

The archive is one process with one local root. This page is what an operator runs and what they
look at when a delivery does not arrive.

## Start

```sh
export ARCHIVE_ROOT=/srv/archive
export ARCHIVE_TOKEN_SECRET="$(cat /etc/archive/token-secret)"
export ARCHIVE_TENANTS=acme,globex
exec archive
```

The service validates the whole configuration before it creates a directory or opens a listener. A
root that is relative, a root that is `/`, a tenant list with a repeated id, a secret shorter than
sixteen bytes, and a retention window of zero days are all refusals, and the message names the value.

## The tree

```
$ARCHIVE_ROOT/
  objects/
    blobs/<ab>/<digest>   the committed bytes, one file per digest
    meta/<id>.json        one metadata record per object
    text/<id>.txt         the text the pipeline extracted
  .staging/<batch>/
    manifest.json         the manifest the batch was accepted with
    committed             written when every entry committed
    scans/<entry>/        the container, page images and page-index.tsv
    sidecars/<entry>/     the text sidecar for a non-scan entry
```

A batch directory is read through the staging layout, which is the only code that turns a name into a
path. `page-index.tsv` is the scanner's own file: one tab-separated line per page, the page number,
the file name relative to the entry's directory, and the SHA-256 of the page image.

## Deliver a batch

The scanner's uploader posts `multipart/form-data` to `/api/batches`: a `manifest` part with the JSON
description, a `container:<entry>` part per entry, `page:<entry>:<n>` parts for a scan and optional
`sidecar:<entry>` parts. The manifest is validated before a part is written.

```sh
curl -fsS -X POST http://127.0.0.1:8080/api/batches \
  -H "Authorization: Bearer $TOKEN" \
  -F manifest=@batch.json \
  -F container:scan-01=@scan-01.pdf \
  -F page:scan-01:1=@page-001.pgm
```

A delivery that fails validation writes nothing. A delivery that fails during the pipeline keeps its
tree, reports the failure per entry, and is finished by posting to its `resume` endpoint once the
cause is fixed.

## Resume after a crash

A process that stops between staging and commit leaves the batch tree and whatever it had committed.
`GET /api/batches/{id}` reports which entries are staged and which committed;
`POST /api/batches/{id}/resume` runs the stages for the entries the store does not carry yet. The
resume reads the pages the index names, because the interruption may have lost the extracted text,
and re-derives the digest the index carries for each page.

## Retention

`POST /api/admin/sweep` removes the closed batches whose commit day is before
`ARCHIVE_RETENTION_DAYS` UTC days ago. The most recent three batches are always kept, and a batch
whose manifest asked to be pinned is kept too. A batch whose objects are still inside the window is
not removed, so a sweep cannot leave objects that nothing explains. The staged trees older than 24
hours are discarded by the same call: they are deliveries nobody finished.

## Integrity

`GET /api/admin/verify` walks the metadata and the blobs and reports three things: a record whose
blob is missing, a blob whose bytes no longer match its digest, and a blob no record carries. It
opens files and compares digests; it never rewrites anything.

## Audit

`GET /api/admin/audit` and `GET /api/admin/console` show the trail and the configuration. Every
mutating request appends an entry — accepted, resumed, aborted, deleted, swept — and every refusal
does too, with the reason the operator needs and the client never sees. The trail is bounded; a
process that has dropped entries says so in its summary.

## What a batch report claims

The report page is the record an operator keeps with a delivery. It prints what the manifest
declared (entry names, titles, kinds, page counts, sizes, digests, provenance), the pipeline's summary
line, and the committed objects with their ids. Every value the sender wrote is rendered as text:
a title, a note and a provenance URI appear on the page exactly as the sender wrote them, escaped so
the browser shows them and does not interpret them.
