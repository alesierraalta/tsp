export const KEEP_ALIVE_MS = 180;

export class RelayReader {
  constructor(client, { batchSize = 32 } = {}) {
    if (!Number.isInteger(batchSize) || batchSize < 1) {
      throw new RangeError('positive batch size required');
    }
    this.client = client;
    this.batchSize = batchSize;
  }

  async next(topic, cursor, signal) {
    const rows = await this.client.read(topic, cursor, {
      blockMs: KEEP_ALIVE_MS,
      limit: this.batchSize,
      signal,
    });
    return rows;
  }

  async flush(subscriber, cursor, signal) {
    await this.client.acknowledge(subscriber, cursor, { signal });
  }
}
