import { conflict, notFound } from '../util/errors.js';
import { newId, idempotencyKey } from '../util/ids.js';

export const STATUSES = ['pending', 'retrying', 'delivered', 'failed'];

function normalise(input, clock) {
  const now = new Date(clock()).toISOString();
  return {
    id: input.id ?? newId('dlv'),
    event: String(input.event ?? 'unknown'),
    topic: String(input.topic ?? 'default'),
    target: String(input.target ?? input.url ?? ''),
    url: String(input.url ?? ''),
    entity_id: input.entity_id === undefined ? undefined : String(input.entity_id),
    payload: input.payload ?? {},
    headers: { ...(input.headers ?? {}) },
    status: input.status ?? 'pending',
    attempts: input.attempts ?? 0,
    next_attempt_at: input.next_attempt_at ?? now,
    created_at: input.created_at ?? now,
    updated_at: now,
    last_error: input.last_error ?? null,
  };
}

export class DeliveryStore {
  constructor(options = {}) {
    this.clock = options.clock ?? (() => new Date());
    this.rows = new Map();
    this.order = [];
  }

  insert(input) {
    if (input === null || typeof input !== 'object') throw new TypeError('a delivery object is required');
    const row = normalise(input, this.clock);
    if (this.rows.has(row.id)) throw conflict(`delivery ${row.id} already exists`);
    const key = idempotencyKey(row);
    for (const existing of this.rows.values()) {
      if (idempotencyKey(existing) === key && existing.status !== 'delivered') {
        throw conflict(`an equivalent delivery is already queued (${existing.id})`);
      }
    }
    this.rows.set(row.id, row);
    this.order.push(row.id);
    return row;
  }

  get(id) {
    const row = this.rows.get(id);
    if (!row) throw notFound(`delivery ${id}`);
    return row;
  }

  find(id) {
    return this.rows.get(id);
  }

  update(id, patch) {
    const row = this.get(id);
    Object.assign(row, patch, { updated_at: new Date(this.clock()).toISOString() });
    return row;
  }

  list(filters = {}) {
    let rows = this.order.map((id) => this.rows.get(id)).filter(Boolean);
    if (filters.status) rows = rows.filter((row) => row.status === filters.status);
    if (filters.topic) rows = rows.filter((row) => row.topic === filters.topic);
    rows.sort((left, right) => (left.created_at < right.created_at ? -1 : 1));
    return rows.slice(0, filters.limit ?? 100);
  }

  due(now = this.clock()) {
    const at = new Date(now).getTime();
    return this.order
      .map((id) => this.rows.get(id))
      .filter((row) => row && (row.status === 'pending' || row.status === 'retrying'))
      .filter((row) => new Date(row.next_attempt_at).getTime() <= at)
      .sort((left, right) => (left.next_attempt_at < right.next_attempt_at ? -1 : 1));
  }

  markAttempt(id, error) {
    const row = this.get(id);
    row.attempts += 1;
    if (error) row.last_error = { message: String(error.message ?? error), code: error.code };
    return this.update(id, { attempts: row.attempts, last_error: row.last_error });
  }

  markDelivered(id, receipt = {}) {
    return this.update(id, {
      status: 'delivered',
      delivered_at: new Date(this.clock()).toISOString(),
      receipt,
      last_error: null,
    });
  }

  markRetrying(id, nextAttemptAt, error) {
    return this.update(id, {
      status: 'retrying',
      next_attempt_at: nextAttemptAt,
      last_error: { message: String(error?.message ?? error), code: error?.code },
    });
  }

  markFailed(id, error) {
    return this.update(id, {
      status: 'failed',
      last_error: { message: String(error?.message ?? error), code: error?.code },
    });
  }

  remove(id) {
    const row = this.get(id);
    this.rows.delete(id);
    this.order = this.order.filter((entry) => entry !== id);
    return row;
  }


  byEntity(entityId) {
    const wanted = String(entityId);
    return this.list().filter((row) => row.entity_id === wanted);
  }

  search(term) {
    const needle = String(term ?? '').toLowerCase();
    if (needle.length === 0) return [];
    return this.list().filter((row) =>
      [row.event, row.topic, row.target, row.url].some((value) => String(value ?? '').toLowerCase().includes(needle)),
    );
  }

  stale(seconds, now = this.clock()) {
    return this.list().filter((row) => isStale(row, seconds, now));
  }

  requeueStale(seconds, now = this.clock()) {
    const rows = this.stale(seconds, now);
    for (const row of rows) {
      this.update(row.id, { status: 'pending', next_attempt_at: new Date(now).toISOString() });
    }
    return rows.map((row) => row.id);
  }

  summary() {
    const rows = this.list();
    return {
      counts: this.counts(),
      oldest: rows[0]?.created_at ?? null,
      newest: rows[rows.length - 1]?.created_at ?? null,
      topics: [...new Set(rows.map((row) => row.topic))].sort(),
      hosts: [...new Set(rows.map((row) => row.target))].sort(),
    };
  }

  deadLetters(limit = 50) {
    return this.list({ status: 'failed', limit });
  }

  clear() {
    this.rows.clear();
    this.order = [];
  }

  count(status) {
    if (!status) return this.rows.size;
    return [...this.rows.values()].filter((row) => row.status === status).length;
  }

  counts() {
    const out = { total: this.rows.size };
    for (const status of STATUSES) out[status] = 0;
    for (const row of this.rows.values()) out[row.status] = (out[row.status] ?? 0) + 1;
    return out;
  }
}

export function deliveryAge(row, now = new Date()) {
  const at = new Date(now).getTime();
  const seen = new Date(row.updated_at ?? row.created_at).getTime();
  return Math.max(0, Math.round((at - seen) / 1000));
}

export function isStale(row, seconds, now = new Date()) {
  return (row.status === 'pending' || row.status === 'retrying') && deliveryAge(row, now) >= seconds;
}

export function deadLetter(row) {
  return {
    id: row.id,
    event: row.event,
    target: row.target,
    attempts: row.attempts,
    last_error: row.last_error,
    created_at: row.created_at,
  };
}
