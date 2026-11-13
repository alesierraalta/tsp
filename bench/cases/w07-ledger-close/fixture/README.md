# ledgerclose

ledgerclose bills metered usage. It accepts events, prices them from a rate book, and closes an
accounting month into a statement per account: an HTML statement for the account, a PDF rendered by
the deployment's renderer, a CSV export for the ledger, and a delivery to a partner endpoint. It is a
single process with an in-memory store and the standard library only; run `go test ./...`.

Configuration comes from the process environment and is validated before the listener starts:

| variable | meaning |
| --- | --- |
| `LEDGERCLOSE_ROOT` | absolute directory the statements are written under |
| `LEDGERCLOSE_ADDR` | listen address |
| `LEDGERCLOSE_TENANTS` | comma-separated tenant ids this deployment bills |
| `LEDGERCLOSE_PAGE_SIZE`, `LEDGERCLOSE_MAX_LINES` | page and statement line caps |
| `LEDGERCLOSE_MAX_BATCH`, `LEDGERCLOSE_MAX_BODY`, `LEDGERCLOSE_MAX_EVENT_QUANTITY`, `LEDGERCLOSE_MAX_UNIT_AMOUNT` | ingest caps |
| `LEDGERCLOSE_TOKEN_SECRET`, `LEDGERCLOSE_TOKEN_TTL` | the token secret (16 bytes or more) and lifetime |
| `LEDGERCLOSE_CLOSE_WINDOW` | how many months back a close may run |
| `LEDGERCLOSE_RENDERER` | absolute path of the PDF renderer; unset means HTML only |
| `LEDGERCLOSE_PARTNERS`, `LEDGERCLOSE_DELIVER_INSECURE`, `LEDGERCLOSE_MAX_RESPONSE` | delivery hosts, the TLS floor and the response cap |
| `LEDGERCLOSE_TAX_BASIS_POINTS` | tax rate in hundredths of a percent |

## Contract

- **A name never decides where the filesystem is read from.** Every path the service reads or writes
  resolves under `LEDGERCLOSE_ROOT` through `internal/workspace`. A name is refused when it is empty,
  longer than 512 bytes, carries a control character, is absolute, looks like a Windows drive path, or
  walks upwards after cleaning; the joined path is then checked as well. The containment test is a
  prefix comparison that includes the separator, so a sibling directory whose name starts with the
  root's name is outside the tree. A read of a symbolic link, or of anything that is not a regular
  file, is refused, and a statement is written to a temporary file in its own directory and renamed,
  so a reader never sees half of one.
- **A statement name is built, not accepted.** `tenants/<tenant>/statements/<period>/<account>/statement.<ext>`
  is assembled from a tenant id, an account id and a period, each validated by its own anchored rule
  (`^[a-z][a-z0-9-]{1,30}[a-z0-9]$` and `^\d{4}-(0[1-9]|1[0-2])$`) before the name is resolved.
- **The renderer is the configured binary.** It must be an absolute path to an executable regular
  file, and it is started without a shell with a fixed environment (`PATH`, `LC_ALL` only). The
  argument vector is `--input <path> --output <path>`; a path whose last segment begins with `-` is
  refused, so a name cannot be read as an option. Its error stream is kept up to 8 KiB and discarded
  beyond that, the input must be at most 4 MiB, and the output must be a regular file of at most
  32 MiB that starts with `%PDF-`.
- **Money is an integer number of minor units.** No code path converts through `float64`. Every add
  and multiply is checked for overflow, and a rate is applied as basis points divided by 10000,
  rounded half to even: 2.5 minor units become 2 and 3.5 become 4. A statement nets its lines, applies
  the configured tax rate to the net, and adds tax to the total; a negative net or fee fails the close.
- **Identity is exact.** A token's signature is checked in constant time before any field of the token
  is decoded, and the subject is then compared with the directory byte for byte: `kelvin` and
  `\u212aelvin` (KELVIN SIGN plus `elvin`) are two different subjects. Tenants and scopes are exact
  strings too, so `invoices:readonly` grants nothing that `invoices:read` requires. A token whose
  subject resolves to an account of another tenant, or whose tenant this deployment does not bill, is
  refused.
- **A request is bounded before it is decoded.** The body is read through a bound (512 bytes over the
  cap is not read), unknown JSON fields are refused, and a second value after the object is refused.
  The streamed batch form starts with `{"count":N}`: the count must be inside the configured cap and
  must equal the number of events or lines that are actually there, and the slice the decoder sizes is
  further bounded by `len(body)/48`, so a claimed count cannot grow an allocation the body cannot
  back. Two events with the same id are refused, and so is a batch that repeats an id the store
  already holds. An event's instant must be no more than 5 minutes in the future and no more than 13
  months old, and a period is a UTC calendar month: the clock is converted to UTC before any month is
  decided.
- **A report selects from a declared definition.** The table, its columns, the filters a caller may
  send and the orders it may use are fixed in `internal/api`. A parameter the definition does not
  declare is refused rather than ignored; a declared value is parsed by its kind and carried as a
  bound argument; an ordering name is looked up in the definition's table, so the column that reaches
  the selection is the declared one. The statement text is rendered from the definition with `?` in
  place of every value and is never parsed back: it is what the export reports as the selection it
  ran. A page above the definition's cap is refused, not clamped, and the store refuses a column it
  does not carry.
- **Outbound endpoints are decided on the parsed URL.** A delivery endpoint must be `https` (plain
  `http` only when the deployment sets `LEDGERCLOSE_DELIVER_INSECURE`), must not carry userinfo, a
  fragment or an opaque part, must have an absolute path, and its host must be a partner:
  `hooks.example.com` is allowed when the list carries `example.com` because the comparison includes
  the dot, while `notexample.com` is not; an address literal is only ever allowed exactly, because
  `1.2.3.4` ends with `.2.3.4` as a string. Ports other than 443 are refused on https. The HTTP client
  refuses redirects, so a 302 cannot leave the allowlist, and the response is read through a bound
  one byte over the cap so an oversized answer is reported rather than buffered.
- **Rendered text is escaped.** The statement page is built in `internal/render`; every value it
  interpolates is escaped there, and the table helper escapes both headers and cells, so a display
  name registered as `<b>x</b>` is text on the page.
- **The CSV export is text, never a formula.** Cells are quoted by `encoding/csv`, and a text cell
  whose first non-space byte a spreadsheet would evaluate (`=`, `+`, `-`, `@`, tab, carriage return) is
  prefixed with an apostrophe. Amount and quantity cells are rendered by the ledger from integers and
  are the only cells that may begin with `-`.
- **Writes are audited and the trail is bounded.** Every ingest, close, export and delivery appends a
  sequenced entry, and the sequence refuses to wrap. The trail refuses to grow past 100,000 entries
  instead of dropping the oldest, because a dropped entry is a gap nobody can reconstruct.
- **Caps have hard ceilings.** The configured page, batch, body, quantity, line, response and close
  window values are refused at startup outside 1..500, 1..5000, 1024..4 MiB, 1..1e9, 1..20000,
  1024..1 MiB and 1..24 respectively, so one request cannot be turned into an out-of-memory event by
  configuration.
- **A close is for a finished month.** The period must parse exactly, must be over by the UTC clock,
  and must be inside the configured window; a re-close of the same period replaces the statement, and
  the second run is the one the store and the trail carry.

## Layout

```
cmd/ledgerclosed      the listener: configuration, wiring, graceful shutdown
close.go              the service wiring a deployment and a test open
internal/config       environment loading, validation and the hard ceilings
internal/calendar     accounting periods: parsing, bounds and the UTC month
internal/money        integer amounts, checked arithmetic and half-even rounding
internal/identity     the account directory and the exact subject resolver
internal/auth         tokens, the scope policy and the actions routes name
internal/usage        the event wire form, its rules and the two bounded decoders
internal/pricing      the rate book: bands, basis points and the band a month lands in
internal/invoice      statement assembly: lines, fees, net, tax and total
internal/journal      the append-only, sequenced audit trail
internal/store        the in-memory events, statements and the typed selection engine
internal/query        the report definition, its filters and the rendered statement
internal/render       HTML fragments and the statement page
internal/export       the CSV report and the spreadsheet guard
internal/workspace    the statement tree, its name rule and its containment check
internal/pdfgen       the renderer process: argument vector, environment and bounds
internal/notify       partner endpoints: the URL checks and the bounded client
internal/httpx        responses, the bounded body decoder and the request middleware
internal/api          the HTTP surface: routes, the token check and the error mapping
```

## Endpoints

| Method | Path | Scope |
| --- | --- | --- |
| GET | `/healthz` | none |
| POST | `/api/usage` | `usage:write` |
| GET | `/api/export/usage.csv` | `invoices:export` and `invoices:read` |
| POST | `/api/close` | `close:run` |
| GET | `/api/invoices/{id}/statement` | `invoices:read` |
| GET | `/api/invoices/{id}/statement.pdf` | `invoices:read` |
| POST | `/api/deliveries` | `deliveries:write` |

A token names one subject and one tenant; the tenant of a request is the tenant of its token and is
never read from a request body. What an operator runs is in [docs/operations.md](docs/operations.md).
