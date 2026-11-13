import { assertTarget, hostOf } from './targets.js';
import { badRequest, conflict } from '../util/errors.js';
import { newId } from '../util/ids.js';
import { nowIso } from '../util/time.js';

export const WEBHOOK_EVENTS = ['release.created', 'deployment.started', 'deployment.finished', 'deployment.rolled_back', 'incident.opened'];

export function normalizeSubscription(input = {}, options = {}) {
  const events = (Array.isArray(input.events) ? input.events : []).filter((event) => WEBHOOK_EVENTS.includes(event));
  if (events.length === 0) throw badRequest('invalid_subscription', `a subscription wants at least one of ${WEBHOOK_EVENTS.join(', ')}`);
  return {
    id: input.id ?? newId('sub'),
    tenant: String(input.tenant ?? options.tenant ?? 'public'),
    label: String(input.label ?? 'relay'),
    events,
    enabled: input.enabled !== false,
    created_at: nowIso(options.clock),
  };
}

export class SubscriptionBook {
  constructor(options = {}) {
    this.clock = options.clock ?? (() => new Date());
    this.allowed = options.allowed;
    this.subscriptions = new Map();
  }

  add(input) {
    const draft = normalizeSubscription(input, { clock: this.clock });
    // The stored url is the one assertTarget approved: it is parsed first, and the host it reaches is the
    // host the policy checked.
    const target = assertTarget(input.url, { allowed: this.allowed });
    const subscription = { ...draft, url: target.url, host: target.host };
    if ([...this.subscriptions.values()].some((entry) => entry.tenant === subscription.tenant && entry.url === subscription.url)) {
      throw conflict('subscription_exists', `${subscription.tenant} already subscribes to ${target.host}`);
    }
    this.subscriptions.set(subscription.id, subscription);
    return subscription;
  }

  get(id) {
    return this.subscriptions.get(String(id));
  }

  require(id) {
    const subscription = this.get(id);
    if (!subscription) throw badRequest('subscription_missing', `subscription ${id} does not exist`);
    return subscription;
  }

  remove(id) {
    return this.subscriptions.delete(String(id));
  }

  enable(id) {
    const subscription = this.require(id);
    const next = { ...subscription, enabled: true };
    this.subscriptions.set(id, next);
    return next;
  }

  disable(id) {
    const subscription = this.require(id);
    const next = { ...subscription, enabled: false };
    this.subscriptions.set(id, next);
    return next;
  }

  list(options = {}) {
    return [...this.subscriptions.values()]
      .filter((subscription) => options.tenant === undefined || subscription.tenant === options.tenant)
      .filter((subscription) => options.event === undefined || subscription.events.includes(options.event))
      .filter((subscription) => options.enabledOnly !== true || subscription.enabled)
      .sort((left, right) => (left.created_at < right.created_at ? -1 : 1));
  }

  targets(tenant, event) {
    return this.list({ tenant, event }).filter((subscription) => subscription.enabled);
  }

  byHost() {
    const out = {};
    for (const subscription of this.subscriptions.values()) out[subscription.host] = (out[subscription.host] ?? 0) + 1;
    return out;
  }

  stats() {
    return {
      subscriptions: this.subscriptions.size,
      enabled: this.list({ enabledOnly: true }).length,
      hosts: Object.keys(this.byHost()).length,
    };
  }

  // A subscription restored from the operator's stored snapshot keeps the url it was saved with. The
  // delivery path checks that url again, on the host it reaches, before it posts anything.
  restore(record = {}) {
    const draft = normalizeSubscription(record, { clock: this.clock });
    const url = String(record.url ?? '');
    if (url === '') throw badRequest('invalid_subscription', 'a restored subscription needs a url');
    const subscription = { ...draft, url, host: hostOf(url) ?? '' };
    this.subscriptions.set(subscription.id, subscription);
    return subscription;
  }

  restoreAll(records) {
    return (records ?? []).map((record) => this.restore(record));
  }
}

export function subscriptionSummary(subscription) {
  return { id: subscription.id, host: subscription.host, events: [...subscription.events], enabled: subscription.enabled };
}
