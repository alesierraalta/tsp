# Operations

## A close

A close runs per tenant and period. It reads the tenant's accounts from the directory, reads each
account's events for the period from the store, assembles one statement per account that has usage, and
writes the statement into the workspace:

```
<root>/tenants/<tenant>/statements/<period>/<account>/statement.html
<root>/tenants/<tenant>/statements/<period>/<account>/statement.pdf   (when a renderer is configured)
```

The period must be finished and inside `LEDGERCLOSE_CLOSE_WINDOW`. A close of a period that is still
running is refused because the month has not finished receiving usage; a close of a period before the
window is refused because that statement was delivered already. A close can be run again, and the
second run replaces the statement and appends its own trail entry.

## The renderer

`LEDGERCLOSE_RENDERER` names an executable that reads an HTML file and writes a PDF. The service starts
it without a shell, passes `--input <absolute path> --output <absolute path>`, and refuses a path whose
last segment begins with `-`. The renderer runs with `PATH=/usr/bin:/bin` and `LC_ALL=C` and nothing
else, so a deployment's secrets are not inherited by it. A renderer that writes a PDF smaller than one
byte, larger than 32 MiB, or without the `%PDF-` signature fails the close for that account.

Without a renderer the close writes the HTML statement and the PDF endpoint answers 404: nothing is
missing, the deployment simply does not render.

## Tokens and scopes

A token is `v1.<payload>.<signature>`, an HMAC-SHA256 over the version and the payload with
`LEDGERCLOSE_TOKEN_SECRET`. The payload carries the subject, the tenant, the scopes and the expiry.
Verification checks the signature before decoding the payload, refuses a token older than its expiry or
longer-lived than the issuer's lifetime, and then resolves the subject against the directory exactly.
Scopes are exact strings, so a token holding `invoices:readonly` cannot export; `invoices:export`
requires both `invoices:export` and `invoices:read`.

## Deliveries and partners

A delivery posts a statement summary as JSON to an endpoint the caller names. The endpoint must be on
the partner list: the parsed host is allowed when the list carries it exactly, or when the list carries
a parent domain and the host is under it. Redirects are refused, so a partner cannot send the request on
to a host the list does not carry. `LEDGERCLOSE_DELIVER_INSECURE` exists for a partner reached over a
private network and for a local test; a deployment that can reach the public internet leaves it off.

## What the service does not do

- It does not resolve symbolic links and follow them. It refuses to read a link at all, and it never
  creates one.
- It does not retry a delivery, a render or an ingest. A refused request is answered and the operator
  decides what to send again.
- It does not stream a statement: a statement is held in memory and bounded by `LEDGERCLOSE_MAX_LINES`.
- It does not keep more than one currency per account, and it does not convert between currencies.
- It does not archive the trail: at 100,000 entries the service refuses the next entry rather than
  dropping the oldest. A store write and its trail entry are not one transaction, so an ingest that
  has stored its batch when the trail is full answers 500 and must not be re-sent (its ids are
  stored), and a delivery that reached the partner is not repeated when its entry could not be
  written, because the request has already left the process.
