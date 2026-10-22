import { checkSignal } from '../lib/signal.js';

export class Session {
  constructor({ reader, permits, metrics, subscriber, topic, cursor, signal }) {
    Object.assign(this, { reader, permits, metrics, subscriber, topic, cursor, signal });
    this.started = false;
  }

  async *stream() {
    if (this.started) throw new Error('session already consumed');
    this.started = true;
    const release = this.permits.acquire(this.subscriber);
    this.metrics.open();
    try {
      while (true) {
        checkSignal(this.signal);
        const rows = await this.reader.next(this.topic, this.cursor, this.signal);
        if (!rows.length) {
          this.metrics.keepAlive();
          yield { type: 'keep-alive', cursor: this.cursor };
        }
        for (const row of rows) {
          checkSignal(this.signal);
          this.cursor = row.id;
          this.metrics.delivered();
          yield { type: 'event', cursor: row.id, topic: row.topic, payload: row.payload };
        }
      }
    } finally {
      try {
        await this.reader.flush(this.subscriber, this.cursor, this.signal);
      } finally {
        release();
        this.metrics.close();
      }
    }
  }
}
