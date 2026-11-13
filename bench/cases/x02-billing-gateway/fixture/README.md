# billing

billing is a multi-tenant billing gateway: it holds the price lists of a tenant's plans, the
subscriptions its accounts run, the usage they report, the invoices those cycles produced and the
payments that settled them. It answers the billing console over HTTP and it is a single process over
an in-memory store with the standard library only. Run `go test ./...`.

Configuration comes from the process environment (`BILLING_ADDR`, `BILLING_TENANTS`,
`BILLING_TOKEN_SECRET`, `BILLING_CURRENCY`, `BILLING_CYCLE_MONTHS`, `BILLING_GRACE_DAYS`,
`BILLING_RETENTION_DAYS`, `BILLING_MAX_PAGE_SIZE`, `BILLING_MAX_BODY`, `BILLING_MAX_INVOICE_ROWS`,
`BILLING_MAX_EVENTS`, `BILLING_TAX_RATE_BASIS`, `BILLING_SESSION_TTL`, `BILLING_ADMIN_SUBJECTS`) and is
validated before the listener starts. The address must parse, there must be at least one tenant, no
tenant may be listed twice, and the token secret must be at least sixteen bytes. See
[docs/operations.md](docs/operations.md) for what an operator runs.

## Contract

- **A caller reads its own tenant.** A token names one subject and one tenant. The tenant of a request
  is the tenant of its token and is never read from a request body or a query parameter. A record is
  resolved only inside that tenant: an identifier that belongs to another tenant is answered as
  not-found, and no response carries a row of a tenant the caller does not name.
- **Every statement is parameterized.** A report is declared once in `internal/reports` as a table, a
  column list, the filters it accepts, an ordering and a row bound. A filter value a caller sends is
  bound as an argument of the statement, so the engine compares it as a value and never parses it as
  part of the statement. A report also declares which filter parameters it accepts, and the values a
  closed filter takes: a parameter the report does not declare, and a value outside the accepted set,
  are refused with `400` rather than ignored.
- **Reports are bounded.** A report without a positive row bound is refused when the registry is built,
  and a request may ask for fewer rows than the deployment's page size and never for more. The CSV
  export carries at most its own declared bound.
- **Money is integer minor units.** An amount is a count of minor units and its currency, and there is
  no float in a stored amount, a line, an invoice or a total: a binary fraction cannot hold a tenth of
  a cent and the error compounds once a million invoices are summed. A list of shares is produced by
  the shared split, whose shares add up to the total exactly.
- **A proration is the shared split of the cycle.** A change made part way through a cycle is priced
  by the number of whole UTC days it covers against the days of that cycle, rounded half up in
  integer arithmetic, and every proration path — a seat credit, a trial conversion, an add-on, a
  cancellation credit, a support retainer — goes through that one computation. A charge and the credit
  that undoes it therefore agree to the minor unit, and the sum of a cycle's prorated lines is the
  cycle's amount.
- **A cycle is whole UTC days.** Cycle boundaries are derived from the subscription's anchor day, not
  from the day of the last invoice; a subscription anchored on the 31st renews on the last day of a
  shorter month and returns to the 31st afterwards. Instants are stored in UTC.
- **An invoice totals its lines.** The store refuses an invoice whose lines do not add to its total and
  one whose paid amount exceeds it. A subscription is billed once per cycle, and an event is billed
  once: an event already carried by an invoice is not charged by a second one.
- **A payment settles an invoice, never more.** A payment above what an invoice owes is refused;
  money the invoice does not ask for is a credit to issue separately. Recording the payment that
  covers the total moves the invoice to `paid`.
- **Authorization is exact.** A token is verified before any of its fields is read, and a scope is the
  string it is: `billing:readonly` grants nothing that `billing:read` requires, and an action the
  policy table does not carry is refused. The operator routes require a scope and a subject the
  deployment names as an operator.
- **Request sizes are capped.** A body is bounded before it is decoded and a body carrying more than
  one object is refused; a listing returns at most the deployment's page size; an invoice may not carry
  more lines, and the store may not hold more usage events, than the deployment accepts.
- **Every mutation is audited.** A mutating request appends to the trail whether it was accepted,
  refused or failed, with the reason the operator needs and the client never sees, and the trail is
  readable only with the audit scope. The trail is bounded and says how many records the bound
  dropped.
- **A rendered value is text.** The console page and the CSV export quote a value that carries a
  comma, a quote or a line break, so a value stays one cell and never becomes a second file column.

## Layout

```
cmd/billingd           the listener: configuration, service, graceful stop
billing.go             the wiring: Options, Open, the clock, the handler
internal/config        environment loading, validation and the request caps
internal/clock         the service's clock, UTC days, cycles and period arithmetic
internal/money         the minor-unit amount, the shared split, proration and formatting
internal/proration     what a change part way through a cycle costs or is worth
internal/invoicing     the lines of an invoice, its totals and its collection schedule
internal/metering      the totals a set of usage events carries, quotas and overage
internal/tax           the rate table, applied in basis points
internal/reconcile     the books checked against themselves
internal/query         the statement builder and the filters a report accepts
internal/sqlrun        the statement engine over the report projection
internal/store         the records and the projection the reports read
internal/reports       the report definitions, the runner and the CSV export
internal/auth         token issue and verify, the scope policy, the principal
internal/audit        the bounded trail and its records
internal/httpx        the segment router, the responses, the classified errors, the bounded reader
internal/httpapi      the routes, the handlers and the views they answer with
internal/pagination   the page and cursor a listing reads and renders
internal/ratelimit    the per-subject allowance
internal/textutil     the labels the console prints
internal/csvx         the CSV writer
```

## Endpoints

| Method | Path | Action |
| --- | --- | --- |
| GET | `/healthz` | none |
| GET | `/api/plans` | `billing.read` |
| GET | `/api/accounts` | `billing.read` |
| GET | `/api/subscriptions` | `billing.read` |
| GET | `/api/subscriptions/{id}` | `billing.read` |
| POST | `/api/subscriptions/{id}/seats` | `billing.write` |
| POST | `/api/subscriptions/{id}/cancel` | `billing.write` |
| GET | `/api/invoices` | `invoices.read` |
| GET | `/api/invoices/{id}` | `invoices.read` |
| POST | `/api/invoices/{id}/payments` | `invoices.write` |
| POST | `/api/usage` | `usage.write` |
| GET | `/api/reports` | `reports.read` |
| GET | `/api/reports/{name}` | `reports.read` |
| GET | `/api/reports/{name}/export.csv` | `reports.export` and `reports.read` |
| GET | `/api/admin/console` | `admin.operate` |
| GET | `/api/admin/audit` | `audit.read` |
| POST | `/api/admin/sweep` | `admin.operate` |

Reports are `usage`, `revenue`, `aging` and `subscriptions`. `GET /api/reports` lists the filters each
one accepts and the values a closed filter takes; a filter value is bound as an argument of the
statement, never written into it. The `usage` report is the one a deployment exports as CSV.

A subscription carries the seats its plan bills above the included count: adding seats part way
through a cycle charges for the days that are left, removing them credits the same part, and a
cancellation credits the days the customer will not use. Every one of those amounts is the shared
proration of the cycle, so the console's figures and the invoice's lines agree to the minor unit.

## Quick start

```sh
BILLING_TENANTS=acme,globex \
BILLING_TOKEN_SECRET=change-me-please-please \
go run ./cmd/billingd --demo
```

The demonstration dataset seeds two tenants, three plans, three accounts, three subscriptions, seven
usage events across the three statuses, and three invoices — one open, one paid and one open in the
second tenant. Without `--demo` the service opens empty and plans, accounts and subscriptions are
loaded through the store's own API before a subscription can be billed.
