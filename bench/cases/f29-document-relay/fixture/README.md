# Document relay

Document relay is an in-process upload spool for a desktop synchronization agent.
Applications enqueue UTF-8 documents under stable identifiers and flush them to an
object endpoint. The queue preserves insertion order. A flush returns committed
receipts, and `status()` exposes pending and delivered counts.

```js
import { Session } from './src/session.js';
import { MemoryEndpoint } from './src/endpoint.js';
const relay = new Session(new MemoryEndpoint(), { namespace: 'team' });
relay.enqueue('notes', 'meeting notes');
await relay.flush();
```

Run `node --test`. There are no external dependencies. The memory adapters implement
the same boundary as production storage and can be supplied asynchronous hooks.
Settings include chunk size, maximum encoded byte length, and total upload attempts.
An identifier can be delivered once per session; pending duplicates are rejected.
An empty document is valid. Reads and snapshots return independent copies.

The endpoint replaces an object atomically when a request completes. A successful
flush must deliver the exact originally enqueued bytes, regardless of transient
endpoint failures. An unsuccessful request cannot publish a partial object.
Storage failures remain observable to the caller and do not relinquish delivery
obligations. See [adapter semantics](docs/adapters.md) for commit ordering, retry
ownership, and receipt persistence guarantees.
