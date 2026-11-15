import { badRequest } from '../util/errors.js';
import { deliveryId } from '../util/ids.js';
import { nowIso } from '../util/time.js';

export const DELIVERY_STATES = ['queued', 'posted', 'failed', 'dropped'];

export function normalizeDelivery(subscription, event, payload, options = {}) {
  if (!subscription?.id) throw badRequest('invalid_delivery', 'a delivery needs a subscription');
  return {
    id: deliveryId(),
    tenant: subscription.tenant,
    subscription: subscription.id,
    host: subscription.host,
    url: subscription.url,
    event,
    payload,
    state: 'queued',
    attempts: 0,
    response_status: null,
    last_error: null,
    queued_at: nowIso(options.clock),
    posted_at: null,
  };
}

export class DeliveryLog {
  constructor(options = {}) {
    this.clock = options.clock ?? (() => new Date());
    this.limit = options.limit ?? 2000;
    this.deliveries = new Map();
  }

  queue(subscription, event, payload) {
    const delivery = normalizeDelivery(subscription, event, payload, { clock: this.clock });
    this.deliveries.set(delivery.id, delivery);
    this.trim();
    return delivery;
  }

  get(id) {
    return this.deliveries.get(String(id));
  }

  require(id) {
    const delivery = this.get(id);
    if (!delivery) throw badRequest('delivery_missing', `delivery ${id} does not exist`);
    return delivery;
  }

  attempt(id, outcome = {}) {
    const delivery = this.require(id);
    const next = {
      ...delivery,
      attempts: delivery.attempts + 1,
      state: outcome.ok === true ? 'posted' : 'failed',
      response_status: outcome.status ?? null,
      last_error: outcome.ok === true ? null : String(outcome.error ?? 'delivery failed'),
      posted_at: outcome.ok === true ? nowIso(this.clock) : delivery.posted_at,
    };
    this.deliveries.set(id, next);
    return next;
  }

  drop(id, reason) {
    const delivery = this.require(id);
    const next = { ...delivery, state: 'dropped', last_error: String(reason ?? 'dropped') };
    this.deliveries.set(id, next);
    return next;
  }

  list(options = {}) {
    return [...this.deliveries.values()]
      .filter((delivery) => options.tenant === undefined || delivery.tenant === options.tenant)
      .filter((delivery) => options.state === undefined || delivery.state === options.state)
      .filter((delivery) => options.host === undefined || delivery.host === options.host)
      .filter((delivery) => options.subscription === undefined || delivery.subscription === options.subscription)
      .sort((left, right) => (left.queued_at < right.queued_at ? 1 : -1))
      .slice(0, options.limit ?? 100);
  }

  pending() {
    return this.list({ state: 'queued', limit: this.deliveries.size });
  }

  byHost() {
    const out = {};
    for (const delivery of this.deliveries.values()) out[delivery.host] = (out[delivery.host] ?? 0) + 1;
    return out;
  }

  byEvent() {
    const out = {};
    for (const delivery of this.deliveries.values()) out[delivery.event] = (out[delivery.event] ?? 0) + 1;
    return out;
  }

  trim() {
    while (this.deliveries.size > this.limit) {
      const oldest = [...this.deliveries.values()].sort((left, right) => (left.queued_at < right.queued_at ? -1 : 1))[0];
      this.deliveries.delete(oldest.id);
    }
    return this.deliveries.size;
  }

  purge(before) {
    const cutoff = new Date(before).getTime();
    let removed = 0;
    for (const [id, delivery] of [...this.deliveries.entries()]) {
      if (new Date(delivery.queued_at).getTime() >= cutoff) continue;
      this.deliveries.delete(id);
      removed += 1;
    }
    return removed;
  }

  stats() {
    const byState = {};
    for (const delivery of this.deliveries.values()) byState[delivery.state] = (byState[delivery.state] ?? 0) + 1;
    return { deliveries: this.deliveries.size, byState, hosts: Object.keys(this.byHost()).length };
  }
}

export function deliverySummary(deliveries) {
  const posted = deliveries.filter((delivery) => delivery.state === 'posted').length;
  const failed = deliveries.filter((delivery) => delivery.state === 'failed').length;
  return {
    deliveries: deliveries.length,
    posted,
    failed,
    success_rate: deliveries.length === 0 ? 0 : Math.round((posted / deliveries.length) * 10000) / 100,
  };
}

export function attemptHistory(delivery) {
  const attempts = Math.max(0, Math.trunc(delivery?.attempts ?? 0));
  return Array.from({ length: attempts }, (_, index) => ({
    attempt: index + 1,
    at: delivery.posted_at ?? delivery.queued_at,
    state: delivery.state,
    status: delivery.response_status,
  }));
}

export function retryCandidates(deliveries, attempts) {
  const cap = Math.max(1, Math.trunc(attempts ?? 4));
  return (deliveries ?? []).filter((delivery) => delivery.state === 'failed' && delivery.attempts < cap);
}

export function successByHost(deliveries) {
  const buckets = new Map();
  for (const delivery of deliveries ?? []) {
    const bucket = buckets.get(delivery.host) ?? { host: delivery.host, posted: 0, failed: 0 };
    if (delivery.state === 'posted') bucket.posted += 1;
    else bucket.failed += 1;
    buckets.set(delivery.host, bucket);
  }
  return [...buckets.values()].sort((left, right) => right.posted - left.posted);
}

export function deliveryTimeline(deliveries) {
  return (deliveries ?? [])
    .map((delivery) => ({ at: delivery.posted_at ?? delivery.queued_at, event: delivery.event, host: delivery.host, state: delivery.state }))
    .sort((left, right) => (left.at < right.at ? 1 : -1));
}

export function stuckDeliveries(deliveries, minutes, clock = () => new Date()) {
  const horizon = new Date(clock()).getTime() - Math.max(0, Math.trunc(minutes)) * 60000;
  return (deliveries ?? []).filter((delivery) => delivery.state === 'queued' && new Date(delivery.queued_at).getTime() < horizon);
}

export function deliveryReport(deliveries) {
  const summary = deliverySummary(deliveries ?? []);
  return { ...summary, byHost: successByHost(deliveries), byEvent: deliveries?.reduce((out, delivery) => ({ ...out, [delivery.event]: (out[delivery.event] ?? 0) + 1 }), {}) ?? {} };
}
