import { limitsFor, remainingCapacity } from '../ingest/limits.js';
import { percentOf } from '../util/numbers.js';

export class UsageLedger {
  constructor(options = {}) {
    this.plans = options.plans;
    this.clock = options.clock ?? (() => new Date());
    this.entries = new Map();
  }

  keyOf(tenant, period) {
    return `${tenant}|${period}`;
  }

  record(tenant, period, usage = {}) {
    const key = this.keyOf(tenant, period);
    const current = this.entries.get(key) ?? { tenant, period, rows: 0, samples: 0, bytes: 0, batches: 0 };
    const next = {
      ...current,
      rows: current.rows + (Number(usage.rows) || 0),
      samples: current.samples + (Number(usage.samples) || 0),
      bytes: current.bytes + (Number(usage.bytes) || 0),
      batches: current.batches + (Number(usage.batches) || 0),
    };
    this.entries.set(key, next);
    return next;
  }

  get(tenant, period = 'current') {
    return this.entries.get(this.keyOf(tenant, period)) ?? { tenant, period, rows: 0, samples: 0, bytes: 0, batches: 0 };
  }

  capacity(tenant, period = 'current') {
    const plan = this.plans.for(tenant);
    const used = this.get(tenant, period);
    const limits = limitsFor(plan);
    return {
      plan: plan.name,
      limits,
      used,
      remaining: remainingCapacity(plan, used),
      percent: {
        rows: percentOf(used.rows, limits.max_rows),
        samples: percentOf(used.samples, limits.max_samples),
        bytes: percentOf(used.bytes, limits.max_bytes),
      },
    };
  }

  warnings(tenant, period = 'current', threshold = 80) {
    const capacity = this.capacity(tenant, period);
    return Object.entries(capacity.percent)
      .filter(([, percent]) => percent >= threshold)
      .map(([metric, percent]) => ({ metric, percent, threshold }));
  }

  reset(tenant, period = 'current') {
    return this.entries.delete(this.keyOf(tenant, period));
  }

  list(tenant) {
    return [...this.entries.values()].filter((entry) => tenant === undefined || entry.tenant === tenant);
  }

  stats() {
    return {
      ledgers: this.entries.size,
      rows: [...this.entries.values()].reduce((total, entry) => total + entry.rows, 0),
    };
  }
}

export function usageSummary(ledger, tenant, period) {
  const capacity = ledger.capacity(tenant, period);
  return {
    plan: capacity.plan,
    batches: capacity.used.batches,
    rows: capacity.used.rows,
    remaining_rows: capacity.remaining.rows,
  };
}

export function overageRows(plan, used) {
  return Math.max(0, Number(used.rows ?? 0) - limitsFor(plan).max_rows);
}
