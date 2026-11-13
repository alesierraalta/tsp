# Operations

## A month's close

A close runs per partner and period. It reads the partner's shipments created inside the period, prices
each one from the rate book, assembles one statement, renders it and writes it into the workspace:

```
<root>/partners/<partner>/statements/<period>/statement.html
```

The period must be finished and inside `SHIPTRACK_CLOSE_WINDOW`. A close of a period that is still
running is refused because the month has not finished receiving shipments; a close of a period before the
window is refused because that statement was delivered already. A close can be run again, and the second
run replaces the statement and appends its own trail entry. `shiptrack.Service.ClosePeriod` runs the same
close a script needs; the route and the method share the period rules, the rate book and the assembly.

## The rate book

A service has weight bands, an amount per billed kilogram, a fuel surcharge in basis points and a
minimum charge. The weight is billed in whole kilograms: 1,500 grams are two kilograms. The band is the
first whose ceiling holds the shipment, so a shipment above the highest band is refused rather than
priced by the last one. The fuel surcharge is the band's rate applied to the base, rounded half to even.
`GET /api/admin/rates` prints the book an operator checks when a partner disputes a line.

## Labels

`SHIPTRACK_LABEL_RENDERER` names an executable that reads a JSON label spec and writes a PDF. The service
starts it without a shell, passes `--input <absolute path> --output <absolute path>`, and refuses a path
whose last segment begins with `-`. The renderer runs with `PATH=/usr/bin:/bin` and `LC_ALL=C` and nothing
else, so a deployment's secrets are not inherited. A renderer that writes a file smaller than the PDF
signature, larger than 32 MiB, or without `%PDF-` fails the request. Without a renderer the label route
answers 404: nothing is missing, the deployment simply does not render.

The input file is written under `<root>/partners/<partner>/labels/<shipment>/label.input.json`; the
artifact lands beside it as `label.pdf`. Both names are built from validated parts and resolved through
the workspace, and the artifact is read back through the workspace before its digest is reported.

## Tokens and scopes

A token is `v1.<payload>.<signature>`, an HMAC-SHA256 over the version and the payload with
`SHIPTRACK_TOKEN_SECRET`. The payload carries the subject, the tenant, the scopes and the expiry.
Verification checks the signature before decoding the payload, refuses a token older than its expiry or
longer-lived than the issuer's lifetime, and then resolves the subject against the directory exactly.
Scopes are exact strings, so a token holding `statements:readonly` cannot read a statement; a close
requires both `statements:close` and `statements:read`.

## Deliveries and partners

A delivery posts a statement notice as JSON to the endpoint registered with the partner. The endpoint is
not part of a request. The host must be on the partner list: the parsed host is allowed when the list
carries it exactly, or when the list carries a parent domain and the host is under it — the comparison
includes the dot, so `example.com` allows `hooks.example.com` and not `notexample.com`. Redirects are
refused, so a partner cannot send the request on to a host the list does not carry.
`SHIPTRACK_DELIVER_INSECURE` exists for a partner reached over a private network and for a local test; a
deployment that can reach the public internet leaves it off. With it on, a host the list carries exactly
may also be reached over plain http on a port other than 80, which is what a partner on a private port
needs; a suffix match gets the default port only.

## Exports and reports

A report is a declared selection. `GET /api/reports` lists the catalog, its parameters and its
selection. An export writes the same selection as CSV under `<root>/exports/<partner>/<report>-<period>.csv`
and answers with the bytes; its header names the selection it ran, with `?` in place of every value. An
export above `SHIPTRACK_MAX_EXPORT_CELLS` is refused rather than truncated. A report's partner filter is
set from the token, so a request cannot read another partner's rows.

## What the service does not do

- It does not resolve symbolic links and follow them. It refuses to read a link at all, and it never
  creates one.
- It does not retry a close, a render or an ingest. A delivery is retried only for a failure a partner
  could recover from — a connection error or a 5xx — and never for a refused host or a 4xx.
- It does not stream a statement: a statement is held in memory and bounded by `SHIPTRACK_MAX_LINES`.
- It does not convert between currencies, and it keeps no price on a shipment: the tariff computes the
  charge at close, so a rate change is one rule and not a migration.
- It does not archive the trail: at `SHIPTRACK_TRAIL_CAP` entries the service refuses the next entry
  rather than dropping the oldest. A store write and its trail entry are not one transaction, so an
  ingest that has stored its batch when the trail is full answers 500 and must not be re-sent (its scans
  are stored), and a delivery that reached the partner is not repeated when its entry could not be
  written, because the request has already left the process.
- It does not keep a scan's payload inside the statement: the payload archive is a file under the tree,
  named by tenant, calendar day and minted event id.
