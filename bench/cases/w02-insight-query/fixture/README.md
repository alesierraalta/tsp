# insight

insight answers the usage questions the billing console asks: metered events per account, archived
events, quota use, and the account list of a tenant. It is a single process with an in-memory store
and the standard library only; run `go test ./...`.

## Contract

- **Every statement is parameterized.** A report is defined once, in `internal/reports`, as a table,
  a column list, an ordering and a row bound. A filter value a caller sends is bound as an argument
  of the statement, so it is compared as a value and never parsed as part of the statement. A
  report also declares which filter parameters it accepts; a parameter it does not declare is
  refused rather than ignored.
- **A caller reads its own tenant.** The tenant of a request is the tenant of the account its token
  resolves to, and every report applies it. Filters narrow that set; no filter can widen it, and no
  route reads a tenant from the query string.
- **Identity resolution is exact.** A token is verified before any of its fields is used, and the
  subject it asserts is matched against the account directory byte for byte: a subject that differs
  in any byte is a different subject. A token whose tenant disagrees with the account's tenant is
  refused.
- **Scopes are a set of exact strings.** `reports:readonly` grants nothing that `reports:read`
  requires, and an action the policy table does not carry is refused.
- **The CSV export is a text file.** Every cell is quoted by the CSV encoder, and the status filter
  accepts only a status the store declares.
- **A report always has a bound.** A definition without a positive row limit is refused when the
  statement is built, so no report reads a whole table.

## Layout

```
cmd/insightd          the listener
insight.go            the wiring: store, directory, registry, HTTP surface
internal/sqlmem       the statement engine: lexer, parser, evaluator, bound arguments
internal/query        the statement builder and the filter helpers
internal/store        the tables, their column lists, and the sample rows
internal/access       tokens, the scope policy, the account directory, the resolver
internal/reports      the report definitions, the runner, the export and the CSV rendering
internal/httpapi      routes, handlers and the error mapping
```

## Endpoints

| Method | Path | Scope |
| --- | --- | --- |
| GET | `/healthz` | none |
| GET | `/api/reports` | `reports:read` |
| GET | `/api/reports/{name}` | `reports:read` |
| GET | `/api/reports/{name}/export.csv` | `reports:export` and `reports:read` |
| GET | `/api/events` | `reports:read` |
| GET | `/api/accounts` | `accounts:read` |
| GET | `/api/quota` | `quotas:read` |

Reports are `usage`, `archive` and `quota`. Each accepts the filters its definition declares — see
`GET /api/reports` for the list a deployment actually serves — and every response carries the
tenant it answered for. The token secret comes from `INSIGHT_TOKEN_SECRET`; a deployment that does
not set one does not start.
