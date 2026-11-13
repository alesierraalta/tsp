# shiptrack

shiptrack tracks shipments for the merchants a logistics provider bills. It accepts a carrier's scans,
books a shipment onto a carrier that serves its lane, closes a merchant's month into a statement, renders
the shipping labels, exports reports as CSV and posts a statement notice to the merchant's endpoint. It
is a single process over a local directory with the standard library only; run `go test ./...`.

Configuration comes from the process environment and is validated before the listener starts:

| variable | meaning |
| --- | --- |
| `SHIPTRACK_ROOT` | absolute directory the statements, labels, exports and event payloads live under |
| `SHIPTRACK_ADDR` | listen address |
| `SHIPTRACK_TENANTS` | comma-separated tenant ids this deployment serves |
| `SHIPTRACK_TOKEN_SECRET`, `SHIPTRACK_TOKEN_TTL` | the token secret (16 bytes or more) and its lifetime |
| `SHIPTRACK_ZONE`, `SHIPTRACK_TAX_BASIS_POINTS` | the reporting zone and the tax rate in hundredths of a percent |
| `SHIPTRACK_PAGE_SIZE`, `SHIPTRACK_MAX_BODY`, `SHIPTRACK_MAX_BATCH` | page, request body and batch caps |
| `SHIPTRACK_MAX_GRAMS`, `SHIPTRACK_MAX_NOTE`, `SHIPTRACK_MAX_LINES` | per-shipment, per-note and per-statement caps |
| `SHIPTRACK_MAX_EVENTS`, `SHIPTRACK_MAX_SHIPMENTS`, `SHIPTRACK_TRAIL_CAP` | how many records the process holds |
| `SHIPTRACK_CLOSE_WINDOW`, `SHIPTRACK_MAX_EXPORT_CELLS`, `SHIPTRACK_MAX_RESPONSE` | close window in months, export budget, delivery response cap |
| `SHIPTRACK_EGRESS_HOSTS`, `SHIPTRACK_DELIVER_INSECURE` | the partner hosts a delivery may reach, and the plain-http allowance |
| `SHIPTRACK_LABEL_RENDERER` | absolute path of the label binary; unset means the label route answers 404 |

## Contract

- **The tree is the only place a path can point.** Every path the service reads, writes or lists
  resolves under `SHIPTRACK_ROOT` through `internal/workspace`: statements, labels, label inputs,
  exports and archived event payloads alike. A name is refused when it is empty, longer than 512 bytes,
  carries a control character, is absolute, looks like a Windows drive path, or walks upwards after
  cleaning; the joined path is then checked as well. The containment test is a prefix comparison that
  **includes the separator** (`root + "/"`), so a sibling directory whose name starts with the root's
  name is outside the tree. That test is short, and it is correct: `filepath.Join` has already cleaned
  the path, so nothing can reach a separator the comparison does not see. Every read is made with
  `Lstat` first: a symbolic link, a directory, a device or a fifo is refused rather than opened, and a
  file above the caller's bound is refused rather than truncated. Statements, labels and exports are
  written to a temporary file in the object's own directory and renamed, so a reader never sees half of
  one. The variable parts of a name are validated by their own anchored rules (a tenant, partner or
  report is a name segment; a shipment or an event is a minted identifier) before the name is built, and
  no handler assembles a name from request text.
- **A request is scoped to the token's partner.** A token is `v1.<payload>.<signature>`, an HMAC-SHA256
  over the version and the payload with `SHIPTRACK_TOKEN_SECRET`. The signature is checked before any
  field of the payload is decoded, and a token that lives longer than the issuer's lifetime is refused.
  The subject is then resolved against the directory **byte for byte**: `partner:shop` names one
  partner, `partner:Shop` names nothing, and a subject of another tenant is refused. The tenant of a
  request is the token's tenant and is never read from a body. A scope is granted by the string it is,
  so `statements:readonly` grants nothing that `statements:read` requires, and a close requires both
  `statements:close` and `statements:read`. A shipment, statement or delivery of another partner is
  answered as missing rather than as forbidden, so a guessed id tells a caller nothing.
- **SQL is rendered from a declared selection, never from a request.** A report is a declaration: a
  table, the columns it shows, the filters it accepts, the orders it allows and its page. The selection
  is validated against the declared schema and rendered with `?` in place of every value, with the
  values returned beside the text as bound arguments; the text is what an export header reports as the
  selection it ran and it never contains a value. A parameter the declaration does not carry is refused
  rather than ignored, an order name is looked up in the declaration's table, and a limit above the
  declaration's cap is refused rather than clamped. A report's partner filter is set from the token
  after the request's parameters are read, so a request cannot widen a report to another partner.
- **Outbound endpoints are decided on the parsed URL.** A delivery endpoint must be `https` (plain
  `http` only when the deployment sets `SHIPTRACK_DELIVER_INSECURE`), must carry no userinfo, fragment
  or opaque part, must have an absolute path, and must name a partner host: `hooks.example.com` is
  allowed when the list carries `example.com` **because the comparison includes the dot**, while
  `notexample.com` and `example.com.evil.test` are not; a host with a trailing dot is refused rather
  than trimmed into a match. An address literal is allowed only when the list carries it exactly,
  because `1.2.3.4` ends with `.2.3.4` as a string. A port other than 443 is refused on https; a plain
  http endpoint on a port other than 80 is allowed only for a host the list carries exactly, which is
  what a private partner on another port is, while a suffix match gets the default port only. The HTTP
  client refuses redirects, so a 302 cannot leave the allowlist, and the response is read through a
  bound one byte over the cap so an oversized answer is reported rather than buffered. The endpoint is
  read from the partner record, never from the request that asks for a delivery.
- **The label binary is run with a fixed argument vector.** `SHIPTRACK_LABEL_RENDERER` must be an
  absolute path to an executable regular file. It is started without a shell with
  `--input <path> --output <path>` and the fixed environment `PATH` and `LC_ALL` only, so the
  deployment's secrets are not inherited. Both paths are absolute and both are refused when their last
  segment begins with `-`, so a name cannot be read as an option. Its output is kept up to 8 KiB per
  stream, the input must be at most 1 MiB, and the artifact must be a regular file of at most 32 MiB
  that starts with `%PDF-` — opened with `Lstat`, so a renderer that wrote a symbolic link is refused.
- **Rendered text is escaped.** Every page is built in `internal/render`, and every value the service
  interpolates — a partner's display name, a lane, a scan note, a report cell, an audit detail — passes
  through `Escape` or `Attr` first. The table helper escapes both headers and cells, so a partner
  registered as `<b>Shop</b>` is text on the page. No other package writes an HTML tag.
- **Money is an integer number of minor units.** No code path converts through `float64`. Every add,
  subtract and multiply is checked for overflow, and a rate is applied as basis points divided by
  10000, **rounded half to even**: 2.5 minor units become 2 and 3.5 become 4, and -2.5 becomes -2. Half
  to even is the rule the rate book and the tax authority agree on, so truncation — which drifts one
  way over a month — is not used. A statement nets its lines, applies the tax rate to the net, and adds
  tax to the total; a negative net or surcharge fails the close.
- **A statement's lines add up to its total.** The fuel surcharge is allocated across the statement's
  lines in proportion to their weight under a ceiling per line — no line is charged more fuel, or more
  tax, than its own freight — and the same rule places the tax. The allocation is exact: every minor
  unit is placed, and a total whose ceilings cannot hold it fails the close instead of being truncated.
  Lines are ordered by scan instant and shipment id, so the same month closes to the same page.
- **A close is for a finished month.** The period must parse exactly as `YYYY-MM`, must be over by the
  UTC clock, and must be inside `SHIPTRACK_CLOSE_WINDOW`. Accounting months are UTC months: the period
  of an instant is decided after converting it to UTC, whatever the reporting zone says. The reporting
  zone is used for one thing only — the calendar day an event payload is archived under — and that day
  is computed in the zone and converted back to an instant before anything is stored, so a cutoff is
  the same instant wherever it is read.
- **A request is bounded before it is decoded.** The body is read under the configured cap, unknown
  JSON fields are refused, and a second value after the object is refused. The streamed batch form is
  `{"count":N,"events":[...]}`: the count must be inside the batch cap and must equal the number of
  events actually present, and the slice the decoder sizes is bounded by `len(body)/48`, so a claimed
  count cannot grow an allocation the body cannot back. A scan older than 90 days or more than 5
  minutes in the future is refused, both bounds inclusive, because an old scan may belong to a closed
  month and a future one is a clock the sender has wrong. Two scans with the same shipment, kind,
  instant and facility are one scan: the second is refused.
- **The CSV export is text, never a formula.** Cells are quoted by `encoding/csv`, and a text cell
  whose first non-space byte a spreadsheet would evaluate (`=`, `+`, `-`, `@`, tab, carriage return) is
  prefixed with an apostrophe. Amount and weight cells are rendered by the report from integers and are
  the only cells that may begin with `-`. An export above the cell budget is refused, not truncated.
- **Writes are audited and the trail is bounded.** Every ingest, assignment, close, label render, export
  and delivery appends a sequenced entry, and the sequence does not wrap. The trail refuses to grow
  past its cap instead of dropping the oldest entry, because a dropped entry is a gap nobody can
  reconstruct.
- **Caps have hard ceilings.** The configured page, body, batch, weight, note, line, response, record,
  close-window, export and trail values are refused at startup outside their ranges (for example page
  1..500, body 1024..4 MiB, batch 1..5000, lines 1..20000, close window 1..24), so one request cannot be
  turned into an out-of-memory event by configuration.

Every one of these rules holds for the second entry point as well as the first: the statement page a
partner fetches is read back through the workspace by the name the close wrote, the bound event payload
is written through the workspace under a name built from validated parts, the label artifact is verified
and then read back through the workspace, and the report's partner filter is set on the server side of
the binder.

## Layout

```
cmd/shiptrack        the listener: configuration, service, graceful stop
shiptrack.go         the service wiring: Open, registration, the close a script runs
internal/config      environment loading, validation and the hard ceilings
internal/clock       instants, accounting months and the zone-anchored calendar day
internal/money       integer amounts, checked arithmetic, half-even rounding, capped allocation
internal/identity    the partner and carrier directory and the exact id rules
internal/auth        tokens, scopes, sessions and the action table
internal/events      the scan wire form, its rules, the bounded decoders and the timeline
internal/store       the in-memory records and the typed selection engine
internal/binder      the declared schema, the selection, the filters and the rendered SQL text
internal/report      the report catalog and the runs
internal/tariff      the rate book: bands, whole-kilogram billing and basis points
internal/statement   statement assembly: lines, allocation, totals and the period rules
internal/render      HTML fragments, the escaped table, the statement page
internal/export      the CSV report and the spreadsheet guard
internal/egress      the partner hosts, the parsed-URL rule and the bounded client
internal/notify      the delivery queue and its records
internal/labels      the renderer process: argument vector, environment and bounds
internal/workspace   the tree, the name rule, containment, atomic writes and bounded reads
internal/httpx       the segment router, responses, middleware and the bounded body reader
internal/api         the HTTP surface: routes, the token check, the handlers and the error mapping
internal/audit       the append-only, sequenced trail
internal/checksum    SHA-256 as the service's identity function
internal/idgen       the minted identifiers
internal/textutil    slug, label, clip and byte/weight formatting rules
```

## Endpoints

| Method | Path | Action |
| --- | --- | --- |
| GET | `/healthz` | none |
| POST | `/api/events` | `events:write` |
| POST | `/api/shipments` | `shipments:write` |
| GET | `/api/shipments` | `shipments:read` |
| GET | `/api/shipments/{id}` | `shipments:read` |
| POST | `/api/shipments/{id}/label` | `shipments:write` |
| POST | `/api/close` | `statements:close` and `statements:read` |
| GET | `/api/statements` | `statements:read` |
| GET | `/api/statements/{period}` | `statements:read` |
| POST | `/api/deliveries` | `deliveries:write` |
| GET | `/api/reports` | `reports:read` |
| GET | `/api/reports/{name}` | `reports:read` |
| GET | `/api/export/{name}.csv` | `reports:export` and `reports:read` |
| GET | `/api/admin/stats` | `admin:operate` |
| GET | `/api/admin/rates` | `admin:operate` |
| GET | `/api/admin/audit` | `audit:read` |

A token names one subject and one tenant; the tenant of a request is the tenant of its token and is
never read from a request body. What an operator runs is in [docs/operations.md](docs/operations.md).

## Quick start

```sh
SHIPTRACK_ROOT=/srv/shiptrack SHIPTRACK_TOKEN_SECRET=change-me-please SHIPTRACK_TENANTS=acme \
  go run ./cmd/shiptrack
```

The rate book ships with `ground`, `express` and `freight` services; a deployment that bills differently
registers its own services at open time. A statement lands under
`<root>/partners/<partner>/statements/<period>/statement.html`.
