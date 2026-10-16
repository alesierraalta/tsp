# payment-ledger

Keeps account balances from the payment events a provider publishes. Run `node --test`.

The provider publishes one event per payment: `{ id, account, amount }`, where `id` identifies the
payment. A broker hands the events to `PaymentConsumer.handle(delivery)` as they arrive, one call per
delivery: `{ deliveryTag, event }`.

**Promise:** each account's balance in the `Ledger` equals the sum of the amounts of the distinct payments
the provider published for it.
