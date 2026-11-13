import { countBy } from '../util/collections.js';
import { chargeCents, rateFor } from './rates.js';

export class Meter {
  constructor(options = {}) {
    this.clock = options.clock ?? (() => new Date());
    this.counters = new Map();
  }

  keyOf(tenant, metric, period) {
    return `${tenant}|${metric}|${period}`;
  }

  add(tenant, metric, units, period = 'current') {
    const key = this.keyOf(tenant, metric, period);
    this.counters.set(key, (this.counters.get(key) ?? 0) + (Number(units) || 0));
    return this.counters.get(key);
  }

  get(tenant, metric, period = 'current') {
    return this.counters.get(this.keyOf(tenant, metric, period)) ?? 0;
  }

  linesFor(tenant, period = 'current') {
    const lines = [];
    for (const [key, units] of this.counters.entries()) {
      const [owner, metric, interval] = key.split('|');
      if (owner !== tenant || interval !== period) continue;
      lines.push({ metric, units, rate_cents: rateFor(metric).cents_per_thousand });
    }
    return lines.sort((left, right) => (left.metric < right.metric ? -1 : 1));
  }

  costCents(tenant, period = 'current') {
    return this.linesFor(tenant, period).reduce((total, line) => total + chargeCents(line.units, line.rate_cents), 0);
  }

  byMetric(tenant, period = 'current') {
    return Object.fromEntries(this.linesFor(tenant, period).map((line) => [line.metric, line.units]));
  }

  reset(tenant, period = 'current') {
    let removed = 0;
    for (const key of [...this.counters.keys()]) {
      const [owner, , interval] = key.split('|');
      if (owner === tenant && interval === period) {
        this.counters.delete(key);
        removed += 1;
      }
    }
    return removed;
  }

  stats() {
    return { counters: this.counters.size };
  }
}

export function meterEvents(events) {
  return countBy(events, (event) => event.name);
}

export function estimatedCost(events) {
  const byName = meterEvents(events);
  let total = 0;
  for (const [metric, units] of byName.entries()) {
    const rate = metric === 'purchase' ? 0 : rateFor('events').cents_per_thousand;
    total += chargeCents(units, rate);
  }
  return total;
}
