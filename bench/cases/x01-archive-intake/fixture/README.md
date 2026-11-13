# archive

archive is an intake service: it takes a delivery of scanned paper records, verifies it against the
manifest the sender wrote, extracts the text of every page, and commits the objects it keeps. It
serves the committed archive over HTTP — a listing, one object per document, the extracted text, the
containers, a CSV export, an operator console — and it is a single process over a local directory
with the standard library only. Run `go test ./...`.

Configuration comes from the process environment (`ARCHIVE_ROOT`, `ARCHIVE_TOKEN_SECRET`,
`ARCHIVE_ADDR`, `ARCHIVE_TENANTS`, `ARCHIVE_PAGE_SIZE`, `ARCHIVE_MAX_UPLOAD`,
`ARCHIVE_MAX_ENTRIES`, `ARCHIVE_MAX_PAGES`, `ARCHIVE_SESSION_TTL`, `ARCHIVE_RETENTION_DAYS`,
`ARCHIVE_ZONE`) and is validated before the listener starts. The root must be absolute and must not
be the filesystem root; the token secret must be at least 16 bytes; there must be at least one
tenant. See [docs/operations.md](docs/operations.md) for what an operator runs.

## Contract

- **The tree is the only place a path can point.** Every path the service reads, writes or lists
  resolves inside the root: the staging tree under `.staging/<batch>/`, the committed objects under
  `objects/`, the audit export and the manifests alike. A name that is empty, carries a NUL byte, is
  absolute, or would leave its directory is refused with `ErrInvalid` or `ErrOutside`, and nothing is
  read or written for it. This holds for a name a request supplies and for a file a page index
  names: the index is input the scanner produced, so the page it points at is opened through the
  layout's own reader, which proves the file sits in the entry's directory.
- **A manifest is validated before anything is written.** The tenant, the creation instant, the entry
  count, every entry name, kind, page count, size, digest, title, note, tag and provenance URI are
  checked when the batch is accepted, and a refused batch leaves the tree and the store untouched.
  A scan must carry a page index that is dense from page 1 and whose digests match the pages.
- **Rendered text is escaped.** The document index, the object page, the import report and the
  operator console escape every value a manifest, a record or a page carries — name, title, tag, note,
  provenance, audit detail — so a stored value is text on the page and never markup. Fragments are
  built in `internal/render` and nowhere else.
- **Authorization is exact.** A token is verified before any of its fields is read. A scope is
  granted by the string it is, so `documents:readonly` grants nothing that `documents:read` requires,
  and an action the policy table does not carry is refused. Tenants are compared byte for byte: two
  ids that differ in any byte are two tenants, and a request's tenant is the token's, never the
  body's.
- **Request sizes are capped.** A list request may ask for fewer rows than the configured page size
  and never for more; an upload above the configured maximum is refused rather than truncated; a
  request body is bounded before it is decoded; a manifest may not declare more entries or pages than
  the deployment accepts.
- **The CSV export shows text, never formulas.** A cell whose first byte a spreadsheet would evaluate
  (`=`, `+`, `-`, `@`) is prefixed so the sheet shows the value and does not run it. The audit export
  follows the same rule.
- **Extraction is bounded.** A page's text is read from the payload of its page image, the pipeline
  reads at most the deployment's page cap per scan, and a page image the index does not name is
  never opened.
- **Writes are audited.** Every mutating handler appends to the trail, including the refusals, and
  the trail is readable only with the audit scope.
- **Retention works in whole UTC days.** A closed batch is removed when its commit day is before the
  configured window's horizon; the most recent batches and the pinned ones are kept whatever their
  age. Instants are stored in UTC and only the operator pages render the reporting zone.

## Layout

```
cmd/archive           the listener: configuration, service, graceful stop
archive.go            the service wiring: Options, Open, Import, Resume, helper writers
internal/config       environment loading, validation and the request caps
internal/manifest     the batch description: schema, parsing, the rules it is held to
internal/staging      the batch working tree: layout, page index, quota, listing
internal/intake       the pipeline: stage, scan, extract, classify, commit, resume, report
internal/store        the committed objects: metadata, content-addressed blobs, text, queries
internal/render       the HTML fragments: escape, page, table, index, object, report, console
internal/httpx        the segment router, responses, middleware, bounded readers, pagination
internal/api          the HTTP surface: handlers, error mapping, audit records
internal/auth         token issue and verify, the scope policy, sessions
internal/audit        the append-only trail and its queries
internal/search       the token index a query is answered from
internal/export       the CSV and the fixed-width table
internal/retention    the whole-UTC-day window a sweep applies
internal/storepath    the containment rule every path is resolved through
internal/checksum     SHA-256 as the archive's identity function
internal/idgen        the prefixed random identifiers
internal/clock        the service's only clock, UTC days and windows
internal/textutil     slug, label, tag and byte/duration formatting rules
```

## Endpoints

| Method | Path | Scope |
| --- | --- | --- |
| GET | `/healthz` | none |
| GET | `/api/documents` | `documents:read` |
| GET | `/api/documents/{id}` | `documents:read` |
| GET | `/api/documents/{id}/text` | `documents:read` |
| GET | `/api/documents/{id}/download` | `documents:read` |
| DELETE | `/api/documents/{id}` | `documents:delete` |
| GET | `/api/search` | `documents:read` |
| GET | `/api/export.csv` | `documents:export` and `documents:read` |
| POST | `/api/batches` | `imports:write` |
| GET | `/api/batches` | `imports:read` |
| GET | `/api/batches/{id}` | `imports:read` |
| POST | `/api/batches/{id}/resume` | `imports:write` |
| DELETE | `/api/batches/{id}` | `imports:write` |
| GET | `/api/batches/{id}/report` | `imports:read` |
| GET | `/api/batches/{id}/files` | `imports:read` |
| GET | `/api/batches/{id}/pages/{page}` | `imports:read` |
| GET | `/api/admin/stats` | `admin:operate` |
| GET | `/api/admin/audit` | `audit:read` |
| GET | `/api/admin/console` | `admin:operate` |
| GET | `/api/admin/verify` | `admin:operate` |
| POST | `/api/admin/sweep` | `admin:operate` |

A token names one subject and one tenant; the tenant of a request is the tenant of its token and is
never read from a request body or a manifest. The upload convention of `POST /api/batches` is the
scanner's: a `manifest` part, a `container:<entry>` part per entry, `page:<entry>:<n>` parts for a
scan, and optional `sidecar:<entry>` parts.

## Quick start

```sh
ARCHIVE_ROOT=/srv/archive ARCHIVE_TOKEN_SECRET=change-me-please go run ./cmd/archive
```

An interrupted batch is staged under `<root>/.staging/<batch>/`; posting to
`/api/batches/{id}/resume` finishes it, and the report page shows what the manifest declared next to
what the pipeline committed.
