# order-consumer

Applies order events from an at-least-once queue to an order store. Run `node --test`.

The queue may deliver a message more than once and may deliver messages out of order. Each message is
`{ id, orderId, version, status }`: `id` is unique per message, and `version` is the order's revision, a
positive integer sent as a decimal string (`"1"`, `"2"`, ... `"10"`, ...).

`OrderConsumer.handle(message)` is called once per delivery and returns:

- `"applied"` when the event became the order's state;
- `"duplicate"` when a message with the same `id` was already applied;
- `"stale"` when the store already holds a newer revision of that order; the store is left unchanged.

When the store throws while applying, `handle` rethrows and the delivery counts as not applied: the queue
redelivers the message, and that redelivery is applied.
