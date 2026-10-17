# Billing retry job

Processes recharge jobs with once-only payment effects and matching account
credits. Amounts entering the job use major currency units; the payment adapter
accepts integer minor units, with 100 minor units per major unit.

A job has a globally unique `id`, an `account`, and an `amount`. `run` returns
`complete` after the account credit is committed, or `retry` when another attempt
is needed. Completed jobs can be submitted again without changing the balance.

The payment and ledger adapters are injected. `MemoryPayments` implements the
adapter's optional idempotency key: repeated calls with the same key return the
original receipt. `MemoryLedger` commits a job's credit and completion together.
Its `accept` callback determines whether a commit can be stored on that attempt.

Run `node --test`.
