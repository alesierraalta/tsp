import { tooLarge, badRequest } from '../util/errors.js';
import { limitsFor } from '../ingest/limits.js';

export class Enforcer {
  constructor(options = {}) {
    this.plans = options.plans;
    this.usage = options.usage;
    this.clock = options.clock ?? (() => new Date());
    this.rejections = [];
  }

  checkBatch(tenant, request = {}) {
    const plan = this.plans.for(tenant);
    const limits = limitsFor(plan);
    const rows = Number(request.rows ?? 0);
    if (!Number.isInteger(rows) || rows < 0) throw badRequest('invalid_rows', 'rows must be a whole number');
    if (rows > limits.max_rows) {
      this.rejections.push({ tenant, reason: 'rows', rows, limit: limits.max_rows });
      throw tooLarge(`${rows} rows exceed the ${limits.max_rows} rows of the ${plan.name} plan`);
    }
    const used = this.usage?.get(tenant, 'current') ?? { rows: 0 };
    if (used.rows + rows > limits.max_rows * 1000) {
      this.rejections.push({ tenant, reason: 'monthly', rows, used: used.rows });
      throw tooLarge(`the ${plan.name} plan has no monthly rows left`);
    }
    return { plan: plan.name, rows, limit: limits.max_rows };
  }

  checkSamples(tenant, samples) {
    const plan = this.plans.for(tenant);
    const limits = limitsFor(plan);
    const count = Number(samples ?? 0);
    if (!Number.isInteger(count) || count < 0) throw badRequest('invalid_samples', 'samples must be a whole number');
    if (count > limits.max_samples) {
      this.rejections.push({ tenant, reason: 'samples', samples: count, limit: limits.max_samples });
      throw tooLarge(`${count} samples exceed the ${limits.max_samples} of the ${plan.name} plan`);
    }
    return count;
  }

  checkBytes(tenant, bytes) {
    const plan = this.plans.for(tenant);
    const limits = limitsFor(plan);
    const size = Number(bytes ?? 0);
    if (size > limits.max_bytes) {
      this.rejections.push({ tenant, reason: 'bytes', bytes: size, limit: limits.max_bytes });
      throw tooLarge(`${size} bytes exceed the ${limits.max_bytes} of the ${plan.name} plan`);
    }
    return size;
  }

  checkAll(tenant, request = {}) {
    this.checkBatch(tenant, request);
    this.checkSamples(tenant, request.samples ?? 0);
    this.checkBytes(tenant, request.bytes ?? 0);
    return true;
  }

  stats() {
    const byReason = {};
    for (const rejection of this.rejections) byReason[rejection.reason] = (byReason[rejection.reason] ?? 0) + 1;
    return { rejections: this.rejections.length, byReason };
  }

  recent(limit = 20) {
    return this.rejections.slice(-limit).reverse();
  }
}

export function enforcementReport(enforcer, tenant) {
  const plan = enforcer.plans.for(tenant);
  return {
    tenant,
    plan: plan.name,
    limits: limitsFor(plan),
    rejections: enforcer.stats(),
  };
}

export function rejectionReport(enforcer, tenant) {
  const rows = enforcer.recent(50).filter((row) => tenant === undefined || row.tenant === tenant);
  const byReason = {};
  for (const row of rows) byReason[row.reason] = (byReason[row.reason] ?? 0) + 1;
  return { tenant, rejections: rows.length, byReason, latest: rows[0] ?? null };
}

export function headroom(plan, used = {}) {
  const limits = limitsFor(plan);
  return {
    plan: plan?.name ?? 'free',
    rows: Math.max(0, limits.max_rows - Number(used.rows ?? 0)),
    samples: Math.max(0, limits.max_samples - Number(used.samples ?? 0)),
    bytes: Math.max(0, limits.max_bytes - Number(used.bytes ?? 0)),
  };
}

export function wouldExceed(plan, request = {}) {
  const limits = limitsFor(plan);
  return {
    rows: Number(request.rows ?? 0) > limits.max_rows,
    samples: Number(request.samples ?? 0) > limits.max_samples,
    bytes: Number(request.bytes ?? 0) > limits.max_bytes,
  };
}

export function enforcementMatrix(plans, request = {}) {
  return plans.map((plan) => ({ plan: plan.name, exceed: wouldExceed(plan, request) }));
}
