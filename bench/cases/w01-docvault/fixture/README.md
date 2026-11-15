# docvault

docvault serves one document tree over HTTP: metadata, previews, downloads, attachments, a CSV
export and an operator surface. It is a single process over a local directory and the standard
library only; run `go test ./...`.

Configuration comes from the process environment (`DOCVAULT_ROOT`, `DOCVAULT_ADDR`,
`DOCVAULT_TENANTS`, `DOCVAULT_PAGE_SIZE`, `DOCVAULT_MAX_UPLOAD`, `DOCVAULT_TOKEN_SECRET`,
`DOCVAULT_SESSION_TTL`) and is validated before the listener starts. The root must be absolute and
must not be the filesystem root; the token secret must be at least 16 bytes; there must be at least
one tenant, and tenant ids are lowercase.

## Contract

- **The tree is the only place a path can point.** Every path the service returns, reads, writes or
  lists resolves inside the configured root. A name that is empty, carries a NUL byte, is absolute,
  or would leave the root is refused with `ErrOutside` or `ErrInvalid`, and nothing is read or
  written for it. This holds for the object, download, thumbnail, attachment and reindex paths
  alike: a caller-supplied name never decides where the filesystem is read from.
- **Rendered text is escaped.** The HTML pages (the document index, the per-document preview and
  the layout around them) escape every value a document carries — its title, its kind, its owner,
  its tags and its body — so a stored value is text on the page and never markup. Fragments are
  built in `internal/htmlx` and nowhere else.
- **Authorization is exact.** A token is verified before any of its fields is read. A scope is
  granted by the string it is, so `documents:readonly` grants nothing that `documents:read`
  requires, and an action the policy table does not carry is refused. Tenants are compared byte for
  byte: two ids that differ in any byte are two tenants.
- **Request sizes are capped.** A list request may ask for fewer rows than the configured page size
  and never for more; an upload above the configured maximum is refused rather than truncated; a
  request body is bounded before it is decoded.
- **The CSV export shows text, never formulas.** A cell whose first byte a spreadsheet would
  evaluate (`=`, `+`, `-`, `@`) is prefixed so the sheet shows the value and does not run it.
- **Attachments are flat.** An attachment name is a single path segment with an extension the
  service knows; a nested or unknown name is refused.
- **Writes are audited.** Every mutating handler appends to the audit trail, including the
  refusals, and the trail is readable only with the audit scope.

## Layout

```
cmd/docvault          the listener: configuration, vault, token issuer
internal/config       environment loading, validation and the request caps
internal/auth         token issue/verify, the scope policy, the session resolver
internal/vault        the object tree, its containment rule, metadata and attachments
internal/htmlx        the HTML fragments: escape, page, table, link
internal/httpx        the segment router, responses, middleware, body decoding
internal/api          the HTTP surface: handlers, error mapping, the audit records
internal/report       the index page, the per-document summary and the CSV export
internal/audit        the append-only trail
```

## Endpoints

| Method | Path | Scope |
| --- | --- | --- |
| GET | `/healthz` | none |
| GET | `/api/documents` | `documents:read` |
| POST | `/api/documents` | `documents:write` |
| GET | `/api/documents/{id}` | `documents:read` |
| POST | `/api/documents/{id}` | `documents:write` |
| DELETE | `/api/documents/{id}` | `documents:delete` |
| GET | `/api/documents/{id}/preview` | `documents:read` |
| GET | `/api/documents/{id}/download` | `documents:read` |
| GET | `/api/documents/{id}/thumbnail` | `attachments:read` |
| GET | `/api/documents/{id}/object` | `documents:read` |
| GET | `/api/attachments` | `attachments:read` |
| GET | `/api/attachments/{name}` | `attachments:read` |
| POST | `/api/attachments/{name}` | `documents:write` |
| GET | `/api/export.csv` | `documents:export` and `documents:read` |
| GET | `/api/admin/stats` | `admin:operate` |
| POST | `/api/admin/reindex` | `admin:operate` |
| POST | `/api/admin/purge` | `admin:operate` |
| GET | `/api/admin/audit` | `audit:read` |

A token names one subject and one tenant; the tenant of a request is the tenant of its token and is
never read from a request body. See [docs/operations.md](docs/operations.md) for what an operator
runs.
