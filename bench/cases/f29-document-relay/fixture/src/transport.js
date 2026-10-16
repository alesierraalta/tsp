import { chunks, describe } from './chunks.js';
import { retry, summarize } from './retry.js';

export class Transport {
  constructor(endpoint, settings) {
    this.endpoint = endpoint;
    this.settings = settings;
    this.events = [];
  }

  async upload(key, bytes) {
    const parts = chunks(bytes, this.settings.chunkSize);
    const cursor = parts.values();
    const events = [];
    const receipt = await retry(
      attempt => this.endpoint.replace(key, cursor, attempt),
      this.settings.attempts,
      event => events.push({ ...event, key }),
    );
    this.events.push(...events);
    return Object.freeze({ ...receipt });
  }

  preview(bytes) {
    return describe(bytes, this.settings.chunkSize);
  }

  history(key) {
    return this.events.filter(event => event.key === key).map(event => ({ ...event }));
  }

  stats() {
    return summarize(this.events);
  }

  clearHistory() {
    const count = this.events.length;
    this.events.length = 0;
    return count;
  }
}
