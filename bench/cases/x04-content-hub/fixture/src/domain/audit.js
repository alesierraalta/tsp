import { newId } from '../util/ids.js';
import { singleLine } from '../util/text.js';

export const AUDITED_ACTIONS = [
  'entry.create',
  'entry.update',
  'entry.publish',
  'entry.unpublish',
  'entry.delete',
  'media.put',
  'media.remove',
  'plugin.load',
  'plugin.enable',
  'plugin.disable',
  'schema.create',
  'schema.update',
  'export.write',
];

export class AuditLog {
  constructor(options = {}) {
    this.clock = options.clock ?? (() => new Date());
    this.limit = options.limit ?? 500;
    this.rows = [];
  }

  record(action, { actor, target, details } = {}) {
    if (!AUDITED_ACTIONS.includes(action)) throw new TypeError(`unknown audit action ${action}`);
    const row = {
      id: newId('aud'),
      action,
      actor: singleLine(actor ?? 'system'),
      target: target === undefined ? null : singleLine(String(target)),
      details: details ?? {},
      at: new Date(this.clock()).toISOString(),
    };
    this.rows.push(row);
    if (this.rows.length > this.limit) this.rows.splice(0, this.rows.length - this.limit);
    return row;
  }

  list({ action, actor, target, limit = 50 } = {}) {
    return this.rows
      .filter((row) => action === undefined || row.action === action)
      .filter((row) => actor === undefined || row.actor === actor)
      .filter((row) => target === undefined || row.target === target)
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
    const count = this.rows.length;
    this.rows = [];
    return count;
  }
}

export function auditSummary(log) {
  const counts = log.counts();
  return {
    rows: log.size(),
    actions: Object.keys(counts).length,
    busiest: Object.entries(counts).sort((left, right) => right[1] - left[1])[0] ?? null,
  };
}
