import { newId } from '../util/ids.js';
import { assertTarget } from './targets.js';
import { nowIso } from '../util/time.js';

export const DELIVERY_EVENTS = ['batch.accepted', 'rollup.ready', 'invoice.issued', 'alert.raised'];

export class DeliveryLog {
  constructor(options = {}) {
    this.clock = options.clock ?? (() => new Date());
    this.subscriptions = new Map();
    this.deliveries = [];
  }

  subscribe(tenant, rawUrl, events, options = {}) {
    const target = assertTarget(rawUrl, options);
    const wanted = (events ?? []).filter((event) => DELIVERY_EVENTS.includes(event));
    if (wanted.length === 0) throw new TypeError('a subscription needs at least one known event');
    const subscription = {
      id: newId('sub'),
      tenant,
      url: target.url,
      host: target.host,
      events: wanted,
      created_at: nowIso(this.clock),
    };
    this.subscriptions.set(subscription.id, subscription);
    return subscription;
  }

  unsubscribe(id) {
    return this.subscriptions.delete(String(id));
  }

  targets(tenant, event) {
    return [...this.subscriptions.values()].filter(
      (subscription) => subscription.tenant === tenant && subscription.events.includes(event),
    );
  }

  async publish(tenant, event, payload) {
    const targets = this.targets(tenant, event);
    const rows = targets.map((target) => {
      const row = {
        id: newId('dlv'),
        tenant,
        subscription: target.id,
        host: target.host,
        event,
        payload,
        status: 'recorded',
        attempts: 1,
        at: nowIso(this.clock),
      };
      this.deliveries.push(row);
      return row;
    });
    return rows;
  }

  retry(id) {
    const row = this.deliveries.find((entry) => entry.id === String(id));
    if (!row) return undefined;
    row.attempts += 1;
    row.status = 'recorded';
    return row;
  }

  list(tenant, limit = 50) {
    return this.deliveries
      .filter((row) => tenant === undefined || row.tenant === tenant)
      .slice(-limit)
      .reverse();
  }

  stats() {
    return { subscriptions: this.subscriptions.size, deliveries: this.deliveries.length };
  }

  attempts(id) {
    const row = this.deliveries.find((entry) => entry.id === String(id));
    if (!row) return [];
    return Array.from({ length: row.attempts }, (_, index) => ({ attempt: index + 1, at: row.at, status: row.status }));
  }

  byHost(tenant) {
    const out = {};
    for (const row of this.list(tenant, this.deliveries.length)) out[row.host] = (out[row.host] ?? 0) + 1;
    return out;
  }

  purge(before) {
    const cutoff = new Date(before).getTime();
    const kept = this.deliveries.filter((row) => new Date(row.at).getTime() >= cutoff);
    const removed = this.deliveries.length - kept.length;
    this.deliveries = kept;
    return removed;
  }

  describe(tenant) {
    return [...this.subscriptions.values()]
      .filter((subscription) => tenant === undefined || subscription.tenant === tenant)
      .map((subscription) => ({ id: subscription.id, host: subscription.host, events: subscription.events }));
  }
}

export function deliverySummary(rows) {
  const byHost = {};
  for (const row of rows) byHost[row.host] = (byHost[row.host] ?? 0) + 1;
  return { rows: rows.length, byHost };
}
