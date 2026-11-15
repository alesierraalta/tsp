# event-ingest

A multi-tenant event ingest and analytics service: API keys per tenant, event batches in JSON or NDJSON,
plan quotas with reservations, hourly/daily rollups, session and cohort reports, billing statements, partner
exports, egress webhooks and a retention sweeper that drops what the plan no longer keeps.

## Running it

```sh
node src/main.js     # PORT, INGEST_ROOT, MAX_BODY_BYTES, MAX_SAMPLES, DEFAULT_PLAN, LOG_LEVEL, REGION
node --test
node bin/replay.mjs events.jsonl --chunk=500 --dry-run
```

## Keys and tenants

A key carries a tenant and a scope set. `ingest:write` may post events, `reports:read` may read reports and
`admin` may change plans, keys, retention and partners. A request with no key, an unknown key, a revoked key
or an expired key is refused with `401 unauthorized`; a key without the scope a route needs is refused with
`403 forbidden`. Routes never read a tenant from the body: the tenant is the key's tenant, so a caller cannot
ask for another tenant's events by naming it.

## Ingest

`POST /v1/events` takes one event, `POST /v1/batch` takes `{"events": [...]}`, and `POST /v1/events` with
`content-type: application/x-ndjson` takes one event per line. An event needs a `name`; a name that a
declared event type lists is validated against that type's fields, and an event id already seen inside the
deduplication window is rejected as a duplicate. `POST /v1/events/append` takes a single line and an
`?offset=` for the sender's own bookkeeping.

Bodies are at most `MAX_BODY_BYTES`; a larger body is refused with `413 payload_too_large`. A batch is
planned before a single event is stored: `planBatch` sums the samples the events really carry, checks the row,
sample and byte limits the tenant's plan allows, and sizes the batch's slot array from that sum. An NDJSON
body is planned the same way, one line at a time, and a line past `max_line_bytes` refuses the whole body.

## Quotas and reservations

Every tenant is on a plan: `free`, `growth` or `scale`, with `max_rows`, `max_samples`, `max_bytes` and
`max_line_bytes`. `GET /v1/limits` reports the plan's limits and the reservations it has open.

A reservation asks for rows and samples ahead of time. Both numbers go through `assertWithinLimits` before
any capacity is allocated, and the slot array a reservation holds is sized by the samples the plan accepts,
never by a number the caller can name on its own: a request past `max_samples` is refused with
`413 payload_too_large` and allocates nothing, and an accepted request holds at most one slot per accepted
sample. `DELETE /v1/reservations/:id` releases a reservation and drops its slots; `GET /v1/reservations`
lists them with `rows`, `samples` and `released_at`.

Usage is accumulated per tenant and period, and `percent.rows`, `percent.samples` and `percent.bytes` report
how much of the plan is spent. A percentage at or past 80 is a warning.

## Events and reports

Events are stored with their tenant, name, timestamp, received timestamp, properties, context and samples.
`GET /v1/events` pages them and filters by `name`; `GET /v1/events/export` streams NDJSON; `GET /v1/counts`
counts them by name.

`/v1/reports/*` builds the read side from the stored events: `rollup` groups by `minute`, `hour` or `day`,
`dense` fills the gaps between two instants, `sessions` cuts the events into sessions with a 30 minute gap,
`funnel`, `cohorts` and `usage` answer the questions the dashboards ask. `GET /v1/reports/export` writes the
rollup rows in `csv`, `json` or `ndjson`, in chunks, and describes what it wrote with a manifest: the report,
the format, the columns, the row count and the byte count.

## Money

Prices come from the rate card: `cents_per_thousand` per metric. Money is accumulated in integer cents:
`chargeCents` rounds one line's charge once, `lineCents` is that whole number for a line, and a document's
total is the sum of its line amounts, so `total_cents` is a whole number of cents and `formatCents` only
formats it. `GET /v1/reports/usage` reports the same lines the statement builder charges, and
`POST /v1/reports/invoices` issues a document with `lines`, `total_cents` and `total`.

## Retention and cleanup

Each plan keeps its events for a number of days (`free` 7, `growth` 30, `scale` 90). The retention worker
sweeps a tenant in four steps — raw events, day partitions, series points and snapshots — and reports what
each step looked at before it removes anything. `POST /admin/retention/plan` runs the same sweep with
`dry_run` and returns the counts without deleting; `POST /admin/retention/run` applies it and writes a
`retention.swept` row to the audit log. A sweep never touches a row newer than the plan's cutoff.

The queue carries the sweeps as `retention` jobs. A scheduled job keeps the instant it should run at and is
released to the queue only when that instant has come.

## Partner exports

A partner is a name, a kind and a fixed field list. The field list is the whole of what a partner may see:
`context.user`, `context.referrer` and `context.session` are refused at definition time with
`400 partner_field_blocked`, and a row is built by reading exactly the declared paths, under the column name
the path maps to. An export writes the rows in `csv`, `json` or `ndjson`, keeps the file under the partner and
records its checksum; `POST /v1/partners/:id/bundles` writes the same rows in chunks and returns the name,
row count and checksum of every chunk.

## Webhooks and egress

A subscription is a URL and the events it wants. The URL is checked with `assertTarget` before it is stored:
the text is parsed, the scheme must be `https`, the hostname must not be an internal address (loopback, the
private ranges, link-local, `localhost`) and it must be on the allowlist (`hooks.example.com`,
`ingest.example.com`, `studio-metrics.example.com`). A refused target is `403 forbidden` with
`egress blocked: <host> ...`. Report forwarding uses the same check. Deliveries are recorded with their host,
their event and their attempt count; `retry` bumps the attempt count of a recorded delivery.

## Storage

`MemoryStore` holds named collections keyed by id and refuses a second insert with the same id.
`PartitionedStore` buckets events by tenant and UTC day and can compact a day or drop the days before an
instant. `SeriesStore` keeps hourly points per tenant, metric and dimension, and can prune or compact them to
days. `SnapshotBook` keeps a bounded set of named snapshots. `AuditLog` records plan changes, key changes,
issued invoices, retention sweeps and partner exports, and a prune keeps the rows at or after the instant it
is given.
