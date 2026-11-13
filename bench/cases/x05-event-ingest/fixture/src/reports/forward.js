import { assertTarget } from '../webhooks/targets.js';
import { newId } from '../util/ids.js';
import { nowIso } from '../util/time.js';
import { forbidden } from '../util/errors.js';

export class ReportForwarder {
  constructor(options = {}) {
    this.clock = options.clock ?? (() => new Date());
    this.sent = [];
    this.allowed = options.allowed;
  }

  inspect(rawUrl) {
    try {
      const target = assertTarget(rawUrl, { allowed: this.allowed });
      return { allowed: true, host: target.host, url: target.url };
    } catch (error) {
      return { allowed: false, host: undefined, reason: error.message };
    }
  }

  forward(tenant, rawUrl, payload = {}) {
    const target = assertTarget(rawUrl, { allowed: this.allowed });
    const record = {
      id: newId('fwd'),
      tenant,
      host: target.host,
      report: payload.report ?? 'rollup',
      events: Number(payload.events ?? 0),
      at: nowIso(this.clock),
      status: 'queued',
    };
    this.sent.push(record);
    return record;
  }

  assertAllowed(rawUrl) {
    const decision = this.inspect(rawUrl);
    if (!decision.allowed) throw forbidden(decision.reason);
    return decision;
  }

  history(tenant, limit = 25) {
    return this.sent
      .filter((record) => tenant === undefined || record.tenant === tenant)
      .slice(-limit)
      .reverse();
  }

  stats() {
    const byHost = {};
    for (const record of this.sent) byHost[record.host] = (byHost[record.host] ?? 0) + 1;
    return { sent: this.sent.length, byHost };
  }
}

export function forwardSummary(records) {
  return { records: records.length, hosts: [...new Set(records.map((record) => record.host))] };
}

export function forwardStatuses(records) {
  const out = {};
  for (const record of records ?? []) out[record.status] = (out[record.status] ?? 0) + 1;
  return out;
}

export function pendingForwards(forwarder, tenant) {
  return forwarder.history(tenant, forwarder.sent.length).filter((record) => record.status === 'queued');
}

export function markForward(forwarder, id, status) {
  const record = forwarder.sent.find((entry) => entry.id === String(id));
  if (!record) return undefined;
  record.status = String(status);
  record.updated_at = nowIso(forwarder.clock);
  return record;
}

export function purgeForwards(forwarder, before) {
  const cutoff = new Date(before).getTime();
  const kept = forwarder.sent.filter((record) => new Date(record.at).getTime() >= cutoff);
  const removed = forwarder.sent.length - kept.length;
  forwarder.sent = kept;
  return removed;
}

export function forwardReport(forwarder, tenant) {
  const records = forwarder.history(tenant, forwarder.sent.length);
  return {
    tenant,
    records: records.length,
    hosts: [...new Set(records.map((record) => record.host))],
    statuses: forwardStatuses(records),
    events: records.reduce((total, record) => total + record.events, 0),
  };
}
