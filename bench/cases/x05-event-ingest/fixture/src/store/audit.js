import { newId } from '../util/ids.js';
import { nowIso } from '../util/time.js';

export const AUDITED = ['batch.accepted', 'batch.rejected', 'plan.changed', 'key.issued', 'key.revoked', 'retention.swept', 'invoice.issued', 'partner.exported', 'webhook.replayed'];

export class AuditLog {
  constructor(options = {}) {
    this.clock = options.clock ?? (() => new Date());
    this.limit = options.limit ?? 1000;
    this.rows = [];
  }

  record(action, { tenant, actor, target, details } = {}) {
    if (!AUDITED.includes(action)) throw new TypeError(`unknown audit action ${action}`);
    const row = {
      id: newId('aud'),
      action,
      tenant: tenant ?? null,
      actor: actor ?? 'system',
      target: target ?? null,
      details: details ?? {},
      at: nowIso(this.clock),
    };
    this.rows.push(row);
    if (this.rows.length > this.limit) this.rows.splice(0, this.rows.length - this.limit);
    return row;
  }

  list({ tenant, action, limit = 50 } = {}) {
    return this.rows
      .filter((row) => tenant === undefined || row.tenant === tenant)
      .filter((row) => action === undefined || row.action === action)
      .slice(-limit)
      .reverse();
  }

  counts() {
    const out = {};
    for (const row of this.rows) out[row.action] = (out[row.action] ?? 0) + 1;
    return out;
  }

  size() {
    return this.rows.length;
  }

  clear() {
    const size = this.rows.length;
    this.rows = [];
    return size;
  }

  // The audit log is read by tenant, by action and by actor; a review asks "who did what in this window"
  // and the retention sweeper asks which rows fell out of the plan's window.
  between(from, to) {
    const start = from === undefined ? -Infinity : new Date(from).getTime();
    const end = to === undefined ? Infinity : new Date(to).getTime();
    return this.rows.filter((row) => {
      const at = new Date(row.at).getTime();
      return at >= start && at <= end;
    });
  }

  byActor(tenant) {
    const out = {};
    for (const row of this.list({ tenant, limit: this.rows.length })) out[row.actor] = (out[row.actor] ?? 0) + 1;
    return out;
  }

  prune(before) {
    const cutoff = new Date(before).getTime();
    const kept = this.rows.filter((row) => new Date(row.at).getTime() >= cutoff);
    const removed = this.rows.length - kept.length;
    this.rows = kept;
    return removed;
  }

  exportRows({ tenant, action } = {}) {
    return this.list({ tenant, action, limit: this.rows.length }).map((row) => ({
      id: row.id,
      at: row.at,
      action: row.action,
      tenant: row.tenant,
      actor: row.actor,
      target: row.target,
    }));
  }
}

export function auditSummary(log) {
  const counts = log.counts();
  const busiest = Object.entries(counts).sort((left, right) => right[1] - left[1])[0];
  return { rows: log.size(), actions: Object.keys(counts).length, busiest: busiest ?? null };
}

export function auditWindow(log, from, to, tenant) {
  const rows = log.between(from, to).filter((row) => tenant === undefined || row.tenant === tenant);
  const actions = {};
  for (const row of rows) actions[row.action] = (actions[row.action] ?? 0) + 1;
  return { rows: rows.length, actions };
}

export function auditActors(log, tenant) {
  return Object.entries(log.byActor(tenant))
    .map(([actor, rows]) => ({ actor, rows }))
    .sort((left, right) => right.rows - left.rows);
}
