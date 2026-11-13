import { addDays, isWithin, toInstant } from '../util/time.js';

export const RETENTION = {
  free: 7,
  growth: 30,
  scale: 90,
};

export function retentionDays(planName) {
  return RETENTION[planName] ?? RETENTION.free;
}

export function cutoffFor(planName, clock = () => new Date()) {
  return addDays(clock(), -retentionDays(planName)).toISOString();
}

export function expired(events, planName, clock = () => new Date()) {
  const cutoff = cutoffFor(planName, clock);
  return (events ?? []).filter((event) => (toInstant(event.at)?.getTime() ?? 0) < (toInstant(cutoff)?.getTime() ?? 0));
}

export function retained(events, planName, clock = () => new Date()) {
  const cutoff = cutoffFor(planName, clock);
  return (events ?? []).filter((event) => (toInstant(event.at)?.getTime() ?? 0) >= (toInstant(cutoff)?.getTime() ?? 0));
}

export class RetentionPolicy {
  constructor(options = {}) {
    this.clock = options.clock ?? (() => new Date());
    this.store = options.store;
    this.plans = options.plans;
  }

  sweep(tenant) {
    const plan = this.plans.for(tenant);
    const cutoff = cutoffFor(plan.name, this.clock);
    const rows = this.store.find('events', (event) => event.tenant === tenant && isWithin(event.at, undefined, cutoff));
    for (const row of rows) this.store.remove('events', row.id);
    return { removed: rows.length, cutoff, plan: plan.name };
  }

  window(tenant) {
    const plan = this.plans.for(tenant);
    return { days: retentionDays(plan.name), cutoff: cutoffFor(plan.name, this.clock) };
  }

  report(tenant, events) {
    const plan = this.plans.for(tenant);
    return {
      plan: plan.name,
      days: retentionDays(plan.name),
      total: events.length,
      expired: expired(events, plan.name, this.clock).length,
    };
  }

  // A sweep is planned before it runs: the plan step counts the rows each tenant would lose and leaves them
  // in place, so an operator reads the report and then asks for the same sweep without dry_run.
  plan(tenant) {
    const plan = this.plans.for(tenant);
    const cutoff = cutoffFor(plan.name, this.clock);
    const rows = this.expiring(tenant, cutoff);
    return { tenant, plan: plan.name, cutoff, days: retentionDays(plan.name), expired: rows.length };
  }

  expiring(tenant, cutoff) {
    return this.store.find('events', (event) => event.tenant === tenant && isWithin(event.at, undefined, cutoff));
  }

  sweepAll(tenants) {
    const runs = (tenants ?? this.tenants()).map((tenant) => this.sweep(tenant));
    return { runs, removed: runs.reduce((total, run) => total + run.removed, 0) };
  }

  tenants() {
    if (!this.store) return [];
    return [...new Set(this.store.all('events').map((event) => event.tenant))].sort();
  }

  needsSweep(tenant) {
    return this.plan(tenant).expired > 0;
  }
}

export function retentionTable() {
  return Object.entries(RETENTION).map(([plan, days]) => ({ plan, days }));
}

export function windowOf(planName, clock = () => new Date()) {
  return { plan: planName, days: retentionDays(planName), cutoff: cutoffFor(planName, clock) };
}

export function planWindows(clock = () => new Date()) {
  return Object.keys(RETENTION).map((planName) => windowOf(planName, clock));
}
