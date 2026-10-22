import { delay, checkSignal, checkpoint } from './signal.js';

export const DEFAULT_SOCKET_TIMEOUT = 180;

export class MessageLog {
  #records = new Map();
  #next = 1;

  append(topic, payload) {
    if (typeof topic !== 'string' || !topic.trim()) {
      throw new TypeError('topic required');
    }
    const record = { id: this.#next++, topic, payload: structuredClone(payload) };
    const rows = this.#records.get(topic) ?? [];
    rows.push(record);
    this.#records.set(topic, rows);
    return record.id;
  }

  after(topic, cursor, limit) {
    return (this.#records.get(topic) ?? [])
      .filter(row => row.id > cursor)
      .slice(0, limit)
      .map(row => structuredClone(row));
  }

  topics() {
    return [...this.#records.keys()].sort();
  }

  size(topic) {
    return (this.#records.get(topic) ?? []).length;
  }

  async read(topic, cursor, { blockMs, limit, signal }) {
    checkSignal(signal);
    const ready = this.after(topic, cursor, limit);
    if (ready.length) return ready;
    await delay(blockMs + 12, signal);
    return this.after(topic, cursor, limit);
  }
}

export class LogClient {
  constructor(server, { socketTimeout = DEFAULT_SOCKET_TIMEOUT } = {}) {
    if (!(socketTimeout > 0)) throw new RangeError('socket timeout required');
    this.server = server;
    this.socketTimeout = socketTimeout;
    this.acknowledged = new Map();
  }

  async read(topic, cursor, options) {
    checkSignal(options.signal);
    const controller = new AbortController();
    const forward = () => controller.abort(options.signal.reason);
    options.signal?.addEventListener('abort', forward, { once: true });
    const timer = setTimeout(() => {
      const error = new Error('socket timeout');
      error.code = 'SOCKET_TIMEOUT';
      controller.abort(error);
    }, this.socketTimeout);
    try {
      return await this.server.read(topic, cursor, {
        ...options,
        signal: controller.signal,
      });
    } finally {
      clearTimeout(timer);
      options.signal?.removeEventListener('abort', forward);
    }
  }

  async acknowledge(subscriber, cursor, { signal } = {}) {
    await checkpoint(signal);
    this.acknowledged.set(subscriber, cursor);
  }
}
