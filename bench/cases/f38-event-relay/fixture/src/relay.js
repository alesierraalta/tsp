import { RelayReader } from './reader.js';
import { PermitPool } from './permits.js';
import { Metrics } from './metrics.js';
import { Session } from './session.js';

function name(value, label) {
  if (typeof value !== 'string' || !value.trim()) {
    throw new TypeError(`${label} required`);
  }
  return value.trim();
}

export class Relay {
  constructor(client, { permits = 1, batchSize = 32 } = {}) {
    this.reader = new RelayReader(client, { batchSize });
    this.permits = new PermitPool(permits);
    this.metrics = new Metrics();
  }

  subscribe(subscriber, topic, { cursor = 0, signal } = {}) {
    subscriber = name(subscriber, 'subscriber');
    topic = name(topic, 'topic');
    if (!Number.isSafeInteger(cursor) || cursor < 0) {
      throw new RangeError('nonnegative cursor required');
    }
    const session = new Session({
      reader: this.reader,
      permits: this.permits,
      metrics: this.metrics,
      subscriber,
      topic,
      cursor,
      signal,
    });
    return session.stream();
  }

  status(subscriber) {
    return {
      permits: this.permits.used(subscriber),
      limit: this.permits.limit,
      ...this.metrics.snapshot(),
    };
  }
}
