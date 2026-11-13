import { newId } from '../util/ids.js';
import { isPast } from '../util/time.js';
import { notFound } from '../util/errors.js';

export const DELIVERY_EVENTS = [
  'entry.created',
  'entry.updated',
  'entry.published',
  'entry.unpublished',
  'media.uploaded',
  'plugin.enabled',
  'export.written',
];

export class DeliveryLog {
  constructor(options = {}) {
    this.clock = options.clock ?? (() => new Date());
    this.rows = [];
    this.subscriptions = new Map();
  }

  subscribe(url, events, options = {}) {
    const wanted = events.filter((event) => DELIVERY_EVENTS.includes(event));
    if (wanted.length === 0) throw new TypeError('a subscription needs at least one known event');
    const id = newId('sub');
    this.subscriptions.set(id, { id, url: String(url), events: wanted, secret: options.secret ?? null });
    return this.subscriptions.get(id);
  }

  unsubscribe(id) {
    return this.subscriptions.delete(String(id));
  }

  targets(event) {
    return [...this.subscriptions.values()].filter((subscription) => subscription.events.includes(event));
  }

  async publish(event, payload) {
    const targets = this.targets(event);
    const rows = targets.map((target) => {
      const row = {
        id: newId('del'),
        subscription: target.id,
        event,
        at: new Date(this.clock()).toISOString(),
        attached_at: new Date(this.clock()).toISOString(),
        delivered_at: new Date(this.clock()).toISOString(),
        attempts: 1,
        status: 'recorded',
        payload,
      };
      this.rows.push(row);
      return row;
    });
    return rows;
  }

  pending(now = this.clock()) {
    return this.rows.filter((row) => row.status === 'recorded' && isPast(row.at, () => now));
  }

  retry(id) {
    const row = this.rows.find((entry) => entry.id === String(id));
    if (!row) throw notFound(`delivery ${id}`);
    row.attempts += 1;
    row.status = 'recorded';
    return row;
  }

  failed() {
    return this.rows.filter((row) => row.status === 'failed');
  }

  list(limit = 50) {
    return this.rows.slice(-limit).reverse().map((row) => ({
      id: row.id,
      event: row.event,
      status: row.status,
      attempts: row.attempts,
      at: row.at,
    }));
  }

  stats() {
    const out = {};
    for (const row of this.rows) out[row.event] = (out[row.event] ?? 0) + 1;
    return { subscriptions: this.subscriptions.size, deliveries: this.rows.length, events: out };
  }
}
