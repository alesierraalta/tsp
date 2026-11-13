import { newId } from '../util/ids.js';

export const AUDIT_ACTIONS = ['queued', 'attempted', 'delivered', 'failed', 'requeued', 'blocked', 'hook'];

export class AuditLog {
  constructor(options = {}) {
    this.clock = options.clock ?? (() => new Date());
    this.limit = options.limit ?? 200;
    this.entries = [];
  }

  record(action, fields = {}) {
    if (!AUDIT_ACTIONS.includes(action)) throw new TypeError(`unknown audit action: ${action}`);
    const entry = {
      id: newId('aud'),
      action,
      at: new Date(this.clock()).toISOString(),
      actor: fields.actor ?? 'relay',
      delivery_id: fields.delivery_id,
      host: fields.host,
      detail: fields.detail,
    };
    this.entries.push(entry);
    if (this.entries.length > this.limit) this.entries.shift();
    return entry;
  }

  recent(count = 20) {
    return this.entries.slice(-count);
  }

  byAction(action) {
    return this.entries.filter((entry) => entry.action === action);
  }

  count(action) {
    if (!action) return this.entries.length;
    return this.byAction(action).length;
  }

  counts() {
    const out = {};
    for (const action of AUDIT_ACTIONS) out[action] = 0;
    for (const entry of this.entries) out[entry.action] = (out[entry.action] ?? 0) + 1;
    return out;
  }

  tail(count = 10) {
    return this.recent(count).map(
      (entry) => `${entry.at} ${entry.action} ${entry.delivery_id ?? '-'} ${entry.host ?? '-'}`,
    );
  }

  clear() {
    this.entries = [];
  }
}

export function auditSummary(log) {
  return Object.entries(log.counts())
    .filter(([, value]) => value > 0)
    .map(([action, value]) => `${action}=${value}`)
    .join(' ');
}
