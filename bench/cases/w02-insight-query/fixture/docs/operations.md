# Operations

## The statement engine

`internal/sqlmem` is the in-process engine the service ships with. It understands one `SELECT` over
one table, comparisons joined by `AND` and `OR` (evaluated left to right), an ordering and a row
limit. It exists so the service has no external database to run; it is not a general SQL engine and
refuses what it does not understand instead of guessing.

`DB.Query(sql, args...)` counts the placeholders in the statement and refuses a call whose argument
count differs: a statement with a value spliced into its text where an argument was expected is a
statement that compares the wrong thing, and the count check turns most of those mistakes into an
error at the call site. `DB.QueryText` is the escape hatch for statements that carry no values at
all; nothing that reads a request parameter may use it.

## Accounts and tokens

An account lives in the `accounts` table and the directory is built from it at start-up, so the
authorization path and the account endpoint read the same rows. A token asserts a subject, a tenant
and a scope list; the resolver maps the subject to the account, and the token's tenant must agree
with the account's tenant.

## Quotas

A quota row is the ceiling of one account in one tenant. `used` is the sum the billing pipeline
books; the service reports the pair as it stands and never recomputes it, so a discrepancy is a
pipeline question rather than a reporting one.

## Reloading

The store is in memory. A restart starts from an empty store, and `-seed=true` puts the sample rows
back for a development instance. A production deployment loads its rows from the billing export
before it serves traffic.
