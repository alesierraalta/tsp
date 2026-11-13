import { DeliveryError } from '../util/errors.js';

export const RETRYABLE_CODES = new Set(['delivery_failed', 'network_error', 'timeout']);

export function classify(error) {
  if (!error) return { retryable: true, reason: 'unknown' };
  if (error.retryable === false) return { retryable: false, reason: error.code ?? 'permanent' };
  if (error.code && RETRYABLE_CODES.has(error.code)) return { retryable: true, reason: error.code };
  return { retryable: true, reason: error.code ?? 'unexpected' };
}

export function nextDelay(attempt, options = {}) {
  const base = options.baseMs ?? 250;
  const max = options.maxMs ?? 30_000;
  const exponent = Math.min(12, Math.max(0, attempt - 1));
  const delay = Math.min(max, base * 2 ** exponent);
  return Math.round(delay);
}

export function nextAttemptAt(attempt, now, options = {}) {
  const at = new Date(now);
  if (Number.isNaN(at.getTime())) throw new TypeError(`not a date: ${now}`);
  return new Date(at.getTime() + nextDelay(attempt, options)).toISOString();
}

export function canRetry(row, config) {
  if (!row) return false;
  if ((row.attempts ?? 0) >= config.maxAttempts) return false;
  return row.status === 'pending' || row.status === 'retrying' || row.status === 'failed';
}

export function scheduleRetry(store, id, error, config, options = {}) {
  const row = store.get(id);
  if (!canRetry(row, config)) {
    store.markFailed(id, error);
    return { scheduled: false, status: 'failed', attempts: row.attempts };
  }
  const at = nextAttemptAt(row.attempts, options.now ?? new Date(), config);
  store.markRetrying(id, at, error);
  return { scheduled: true, status: 'retrying', next_attempt_at: at, attempts: row.attempts };
}

export function exhaustedError(row) {
  return new DeliveryError(`delivery ${row.id} failed after ${row.attempts} attempts`, {
    attempts: row.attempts,
  });
}

export function describeBackoff(config = {}) {
  const steps = [];
  for (let attempt = 1; attempt <= (config.maxAttempts ?? 5); attempt += 1) {
    steps.push(nextDelay(attempt, config));
  }
  return steps;
}

export function backoffTable(config = {}, steps = 6) {
  const table = [];
  for (let attempt = 1; attempt <= steps; attempt += 1) {
    table.push({ attempt, delayMs: nextDelay(attempt, config) });
  }
  return table;
}

export function describePolicy(config = {}) {
  return `maxAttempts=${config.maxAttempts ?? 5} base=${config.baseMs ?? 250}ms max=${config.maxMs ?? 30000}ms`;
}

export function withJitter(delayMs, random = Math.random) {
  const factor = 0.5 + random() * 0.5;
  return Math.round(delayMs * factor);
}

export function giveUpAfter(error, attempt, config) {
  if (attempt >= config.maxAttempts) return true;
  return error?.retryable === false;
}

export function retrySummary(row) {
  if (!row) return 'unknown delivery';
  return `${row.id} ${row.status} attempts=${row.attempts} next=${row.next_attempt_at ?? 'none'}`;
}

export function attemptBudget(config = {}) {
  let total = 0;
  for (let attempt = 1; attempt < (config.maxAttempts ?? 5); attempt += 1) {
    total += nextDelay(attempt, config);
  }
  return total;
}
