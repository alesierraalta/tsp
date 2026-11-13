import { badRequest } from '../util/errors.js';

export const DEFAULT_LIMITS = {
  max_rows: 5000,
  max_samples: 100000,
  max_bytes: 4 * 1024 * 1024,
  max_line_bytes: 64 * 1024,
};

export function limitsFor(plan) {
  return { ...DEFAULT_LIMITS, ...(plan?.limits ?? {}) };
}

export function assertWithinLimits(plan, request = {}) {
  const limits = limitsFor(plan);
  const rows = Number(request.rows ?? 0);
  const samples = Number(request.samples ?? 0);
  const bytes = Number(request.bytes ?? 0);
  if (!Number.isInteger(rows) || rows < 0) throw badRequest('invalid_rows', 'rows must be a whole number');
  if (!Number.isInteger(samples) || samples < 0) throw badRequest('invalid_samples', 'samples must be a whole number');
  if (rows > limits.max_rows) {
    throw badRequest('batch_too_large', `${rows} rows exceed the ${limits.max_rows} this plan accepts`, { limit: limits.max_rows });
  }
  if (samples > limits.max_samples) {
    throw badRequest('batch_too_large', `${samples} samples exceed the ${limits.max_samples} this plan accepts`, {
      limit: limits.max_samples,
    });
  }
  if (bytes > limits.max_bytes) {
    throw badRequest('payload_too_large', `${bytes} bytes exceed the ${limits.max_bytes} this plan accepts`, {
      limit: limits.max_bytes,
    });
  }
  return limits;
}

export function planBatch(plan, batch = {}) {
  const events = Array.isArray(batch.events) ? batch.events : [];
  const samples = events.reduce((total, event) => total + (Array.isArray(event?.samples) ? event.samples.length : 0), 0);
  const bytes = Number(batch.bytes ?? Buffer.byteLength(JSON.stringify(events), 'utf8'));
  const limits = assertWithinLimits(plan, { rows: events.length, samples, bytes });
  return {
    accepted: events.length,
    samples,
    bytes,
    slots: new Float64Array(samples),
    limits,
  };
}

export function limitReport(plan) {
  const limits = limitsFor(plan);
  return {
    plan: plan?.name ?? 'free',
    rows: limits.max_rows,
    samples: limits.max_samples,
    bytes: limits.max_bytes,
  };
}

export function remainingCapacity(plan, used = {}) {
  const limits = limitsFor(plan);
  return {
    rows: Math.max(0, limits.max_rows - Number(used.rows ?? 0)),
    samples: Math.max(0, limits.max_samples - Number(used.samples ?? 0)),
    bytes: Math.max(0, limits.max_bytes - Number(used.bytes ?? 0)),
  };
}

export function exceedsAny(plan, request) {
  try {
    assertWithinLimits(plan, request);
    return undefined;
  } catch (error) {
    return error.code;
  }
}
