# Event relay

A dependency-free Node ESM relay for topic-based event subscriptions. Run `node --test`.
Create a MessageLog and LogClient from lib/message-log.js, then pass the client to
Relay from src/relay.js. subscribe(subscriber, topic, options) returns an async iterator.
Events carry a monotonically increasing cursor; resume excludes previously delivered records.

A quiet subscription stays open and receives a keep-alive every K milliseconds
(K = 180 ms, give or take normal scheduling latency). Closing or cancelling
a subscription always returns its permit. Cancellation uses an AbortSignal and propagates
its reason to the caller. Each subscriber has one permit by default; other subscribers
have independent capacity. Return the iterator when a consumer no longer needs it.

See docs/subscriptions.md for delivery and transport semantics, and lib/README.md for
the vendored message-log library's defaults. src/frames.js adapts frames to event-stream text.
