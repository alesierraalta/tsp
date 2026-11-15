import { addSeconds, diffSeconds } from '../util/time.js';

export const DEFAULT_POLICY = { attempts: 4, baseSeconds: 15, factor: 2, maxSeconds: 900 };

export function normalizePolicy(input = {}) {
  const attempts = Math.max(1, Math.min(10, Math.trunc(Number(input.attempts ?? DEFAULT_POLICY.attempts))));
  const baseSeconds = Math.max(1, Math.trunc(Number(input.baseSeconds ?? DEFAULT_POLICY.baseSeconds)));
  const factor = Number.isFinite(Number(input.factor)) && Number(input.factor) >= 1 ? Number(input.factor) : DEFAULT_POLICY.factor;
  const maxSeconds = Math.max(baseSeconds, Math.trunc(Number(input.maxSeconds ?? DEFAULT_POLICY.maxSeconds)));
  return { attempts, baseSeconds, factor, maxSeconds };
}

export function delaySeconds(policy, attempt) {
  const normalized = normalizePolicy(policy);
  const index = Math.max(0, Math.trunc(attempt) - 1);
  return Math.min(normalized.maxSeconds, Math.round(normalized.baseSeconds * normalized.factor ** index));
}

export function nextAttemptAt(policy, attempt, from) {
  return addSeconds(from, delaySeconds(policy, attempt)).toISOString();
}

export function attemptsFor(policy, from) {
  const normalized = normalizePolicy(policy);
  const out = [];
  let cursor = new Date(from ?? Date.now()).toISOString();
  for (let attempt = 1; attempt <= normalized.attempts; attempt += 1) {
    cursor = nextAttemptAt(normalized, attempt, cursor);
    out.push({ attempt, at: cursor, delay_seconds: delaySeconds(normalized, attempt) });
  }
  return out;
}

export function describePolicy(policy) {
  const normalized = normalizePolicy(policy);
  return { attempts: normalized.attempts, window_seconds: normalized.maxSeconds, schedule: attemptsFor(normalized, new Date(0).toISOString()) };
}

export function isExhausted(policy, attempt) {
  return Math.trunc(attempt) >= normalizePolicy(policy).attempts;
}

export function secondsUntil(instant, clock = () => new Date()) {
  return Math.max(0, diffSeconds(new Date(clock()), instant));
}

export class RetryTracker {
  constructor(options = {}) {
    this.policy = normalizePolicy(options.policy);
    this.clock = options.clock ?? (() => new Date());
    this.attempts = new Map();
  }

  record(deliveryId, error) {
    const attempt = (this.attempts.get(deliveryId)?.length ?? 0) + 1;
    const rows = this.attempts.get(deliveryId) ?? [];
    rows.push({ attempt, error: String(error ?? 'failed'), at: new Date(this.clock()).toISOString() });
    this.attempts.set(deliveryId, rows);
    return { attempt, exhausted: isExhausted(this.policy, attempt), next: isExhausted(this.policy, attempt) ? undefined : nextAttemptAt(this.policy, attempt, this.clock()) };
  }

  countFor(deliveryId) {
    return this.attempts.get(deliveryId)?.length ?? 0;
  }

  history(deliveryId) {
    return [...(this.attempts.get(deliveryId) ?? [])];
  }

  stats() {
    return { deliveries: this.attempts.size, attempts: [...this.attempts.values()].reduce((total, rows) => total + rows.length, 0) };
  }
}

export function scheduleTable(policy, from) {
  return attemptsFor(policy, from).map((row) => ({ ...row, minutes: Math.round((row.delay_seconds / 60) * 100) / 100 }));
}

export function totalWindowSeconds(policy) {
  const normalized = normalizePolicy(policy);
  return attemptsFor(normalized, new Date(0).toISOString()).reduce((total, row) => total + row.delay_seconds, 0);
}

export function shouldRetry(policy, attempt, state) {
  if (state !== 'failed') return false;
  return !isExhausted(policy, attempt);
}

export function describeTracker(tracker) {
  return { policy: tracker.policy, stats: tracker.stats() };
}

export function exhaustedIds(tracker) {
  return [...tracker.attempts.entries()]
    .filter(([, rows]) => rows.length >= tracker.policy.attempts)
    .map(([delivery]) => delivery);
}
