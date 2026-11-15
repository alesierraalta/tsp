# Operations

The billing gateway is one process with an in-memory store. This page is what an operator runs, what
they look at when a number does not match, and what a restart costs.

## Start

```sh
export BILLING_ADDR=:8080
export BILLING_TENANTS=acme,globex
export BILLING_TOKEN_SECRET="$(cat /etc/billing/token-secret)"
export BILLING_CURRENCY=EUR
export BILLING_CYCLE_MONTHS=1
exec billingd --demo
```

The service validates the whole configuration before it opens a listener. A relative address, an empty
tenant list, a tenant listed twice, a secret shorter than sixteen bytes and a grace period outside
0..90 days are refusals, and the message names the variable.

Without `--demo` the store starts empty. Plans, accounts and subscriptions are written through the
store's API before the first cycle can be billed; a deployment that starts empty and receives a usage
report has nothing to record it against and answers `422`.

## The store

The records live in memory and the report projection is rebuilt whenever a record changes. A restart
therefore drops the store; a deployment that must survive one runs behind a process supervisor that
reloads the tenant's price list and subscriptions at start-up, and the audit trail is the record of what
the previous process performed.

```
tenants          one per customer organization
plans            the price list: base price, seat price, unit price, included seats and quantity
accounts         one per paying customer inside a tenant
subscriptions    one per account's use of one plan, with its seats and its anchor day
usage_events     one per reported quantity, with its status: recorded, billed or archived
invoices         one per billed cycle, with its lines, its total and what was paid
credits          money an account is owed
payments         money received against an invoice
```

## The cycle

A subscription stores its anchor day once. Every cycle boundary is derived from it: a subscription
anchored on the 31st renews on the last day of a shorter month and returns to the 31st afterwards.
Instants are UTC and only the response formats a date, so a change made at 23:50 on the last day of a
cycle is priced with that day included.

## A change part way through a cycle

Adding seats charges the plan's seat price for the seats above what the plan includes, over the days
that are left in the cycle. Removing them credits the same part. A cancellation credits the part of the
cycle's price the customer will not use. Each of those amounts is the shared proration of the cycle, so
the credit for removing a seat and the charge for adding it back agree to the minor unit, and the sum
of a cycle's prorated lines is the cycle's amount.

The mid-cycle charge is billed on an invoice of its own rather than appended to the cycle's invoice:
an issued invoice is a record the customer has already read.

## Invoices and payments

An invoice carries the plan's base price, the seats above the included count, the metered overage, any
change lines, the tax on that base and the credits the account carries, and its total is the sum of its
lines. The store refuses an invoice whose lines do not add to its total and one whose paid amount
exceeds it.

Recording a payment applies it to the invoice. When the payments cover the total the invoice becomes
`paid`; a payment above the total is refused, because money the invoice does not ask for is a credit to
issue, not a correction to hide inside a paid bill.

## Collection

`POST /api/admin/sweep` moves the tenant's invoices past their grace period (`BILLING_GRACE_DAYS`, seven
by default) to past due and moves the subscription that carries one to `past_due`. The call also
reconciles the tenant's books: the lines of every invoice against its total, the recorded payments
against what each invoice owes, and the invoiced totals against the payments received. A finding is a
disagreement an operator reads; the call changes nothing except the invoices it moved.

The collection schedule is a reminder on the due date, a week later, two weeks later and a final notice
after three weeks. The sweep does not send a reminder itself: it reports which invoices are due, how
many days overdue they are and which reminders the schedule has reached.

## Reports and the console

`GET /api/reports` is the catalog: each report's columns, the filters it accepts and the values a closed
filter takes. A filter parameter the report does not declare, and a value outside the accepted set, are
refused with `400`; a report never widens past the caller's tenant.

`GET /api/admin/console` prints the configuration with the signing secret replaced by a length marker,
the record counts, the routes the process serves and the trail's own summary. `GET /api/admin/audit`
prints the tenant's trail: every accepted, refused and failed mutation with the reason the operator
needs. Every mutating request appends one record.

## Rate limits and tokens

Each subject has an allowance refilled over a minute; a subject above it receives `429` with a
`Retry-After` header. A token carries one subject, one tenant and its scopes, and its lifetime may not
exceed the deployment's session TTL. A scope is the string it is: `billing:readonly` grants nothing
that `billing:read` requires. The operator routes require the operator scope *and* a subject the
deployment lists in `BILLING_ADMIN_SUBJECTS`.
