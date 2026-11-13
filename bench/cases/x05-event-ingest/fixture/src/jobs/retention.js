import { addDays, dayBucket, nowIso, toInstant } from '../util/time.js';
import { cutoffFor, retentionDays } from '../rollups/retention.js';
import { chunk } from '../util/collections.js';

// The retention worker is what runs the plan's retention window: it drops the raw events the window no
// longer covers, the day partitions and the series points built from them, and the snapshots older than
// the window. Every step reports what it looked at before it removes anything, so a sweep can be planned
// with dry_run and only then applied.
export const SWEEP_STEPS = ['events', 'partitions', 'series', 'snapshots'];

export function sweepWindow(planName, clock = () => new Date()) {
  const days = retentionDays(planName);
  const cutoff = cutoffFor(planName, clock);
  return { days, from: addDays(clock(), -days).toISOString(), cutoff };
}

export function expiredRows(rows, cutoff) {
  const limit = toInstant(cutoff)?.getTime() ?? 0;
  return (rows ?? []).filter((row) => (toInstant(row.at)?.getTime() ?? 0) < limit);
}

export function partitionDaysBefore(days, cutoff) {
  const limit = String(dayBucket(cutoff) ?? '');
  return (days ?? []).filter((day) => String(day) < limit);
}

export function snapshotNamesBefore(snapshots, cutoff) {
  const limit = toInstant(cutoff)?.getTime() ?? 0;
  return (snapshots ?? [])
    .filter((snapshot) => (toInstant(snapshot.at)?.getTime() ?? 0) < limit)
    .map((snapshot) => snapshot.name);
}

export class RetentionWorker {
  constructor(options = {}) {
    this.clock = options.clock ?? (() => new Date());
    this.plans = options.plans;
    this.store = options.store;
    this.partitions = options.partitions;
    this.series = options.series;
    this.snapshots = options.snapshots;
    this.audit = options.audit;
    this.batch = options.batch ?? 500;
    this.runs = [];
  }

  window(tenant) {
    return sweepWindow(this.plans.for(tenant).name, this.clock);
  }

  expiredEvents(tenant, cutoff) {
    if (!this.store) return [];
    return expiredRows(this.store.find('events', (row) => row.tenant === tenant), cutoff);
  }

  sweep(tenant, options = {}) {
    const plan = this.plans.for(tenant);
    const window = sweepWindow(plan.name, this.clock);
    const steps = (options.steps ?? SWEEP_STEPS).filter((step) => SWEEP_STEPS.includes(step));
    const dry = options.dry_run === true;
    const out = {
      tenant,
      plan: plan.name,
      days: window.days,
      cutoff: window.cutoff,
      dry_run: dry,
      at: nowIso(this.clock),
      steps: {},
      removed: 0,
    };

    if (steps.includes('events')) {
      const expired = this.expiredEvents(tenant, window.cutoff);
      const batches = chunk(expired, this.batch).length;
      if (!dry) for (const row of expired) this.store.remove('events', row.id);
      out.steps.events = { expired: expired.length, batches };
      out.removed += expired.length;
    }

    if (steps.includes('partitions') && this.partitions) {
      const days = partitionDaysBefore(this.partitions.days(tenant), window.cutoff);
      let dropped = 0;
      if (!dry) for (const day of days) dropped += this.partitions.drop(tenant, day) ? 1 : 0;
      out.steps.partitions = { expired: days.length, dropped: dry ? 0 : dropped, days };
      out.removed += dry ? 0 : dropped;
    }

    if (steps.includes('series') && this.series) {
      const before = this.series.stats().points;
      const removed = dry ? 0 : this.series.prune(window.cutoff);
      out.steps.series = { points: before, removed };
      out.removed += removed;
    }

    if (steps.includes('snapshots') && this.snapshots) {
      const names = snapshotNamesBefore(this.snapshots.list(), window.cutoff);
      let removed = 0;
      if (!dry) for (const name of names) removed += this.snapshots.drop(name) ? 1 : 0;
      out.steps.snapshots = { expired: names.length, removed: dry ? 0 : removed };
      out.removed += dry ? 0 : removed;
    }

    if (!dry) {
      this.runs.push(out);
      this.audit?.record('retention.swept', { tenant, actor: 'worker', target: plan.name, details: { removed: out.removed } });
    }
    return out;
  }

  sweepAll(options = {}) {
    const tenants = options.tenants ?? this.tenants();
    const runs = tenants.map((tenant) => this.sweep(tenant, options));
    return { runs, removed: runs.reduce((total, run) => total + run.removed, 0) };
  }

  tenants() {
    if (!this.store) return [];
    return [...new Set(this.store.all('events').map((row) => row.tenant))].sort();
  }

  plan(tenant, options = {}) {
    return this.sweep(tenant, { ...options, dry_run: true });
  }

  schedule(queue, tenant, options = {}) {
    if (!queue) throw new TypeError('a queue is required to schedule a sweep');
    return queue.enqueue('retention', { tenant, steps: options.steps ?? SWEEP_STEPS }, { priority: options.priority ?? 1 });
  }

  history(tenant) {
    return this.runs.filter((run) => tenant === undefined || run.tenant === tenant);
  }

  stats() {
    return {
      runs: this.runs.length,
      removed: this.runs.reduce((total, run) => total + run.removed, 0),
      tenants: new Set(this.runs.map((run) => run.tenant)).size,
    };
  }
}

export function sweepSummary(runs) {
  const byPlan = {};
  let removed = 0;
  for (const run of runs ?? []) {
    byPlan[run.plan] = (byPlan[run.plan] ?? 0) + run.removed;
    removed += run.removed;
  }
  return { runs: (runs ?? []).length, removed, byPlan };
}

export function retentionQueueJob(worker, tenant) {
  const planned = worker.plan(tenant);
  return {
    kind: 'retention',
    tenant,
    cutoff: planned.cutoff,
    steps: Object.entries(planned.steps).map(([step, details]) => ({ step, expired: details.expired ?? 0 })),
  };
}
