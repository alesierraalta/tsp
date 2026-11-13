import { newId } from '../util/ids.js';
import { nowIso } from '../util/time.js';

export const EVENT_TOPICS = ['release.created', 'deployment.started', 'deployment.finished', 'deployment.rolled_back', 'approval.granted', 'webhook.received', 'incident.opened'];

export class EventLog {
  constructor(options = {}) {
    this.clock = options.clock ?? (() => new Date());
    this.limit = options.limit ?? 5000;
    this.rows = [];
  }

  append(topic, payload = {}, options = {}) {
    if (!EVENT_TOPICS.includes(topic)) throw new TypeError(`unknown event topic ${topic}`);
    const row = {
      id: newId('evt'),
      topic,
      tenant: options.tenant ?? null,
      actor: options.actor ?? 'system',
      subject: options.subject ?? null,
      payload,
      at: nowIso(this.clock),
    };
    this.rows.push(row);
    if (this.rows.length > this.limit) this.rows.splice(0, this.rows.length - this.limit);
    return row;
  }

  list(options = {}) {
    const limit = options.limit ?? 50;
    return this.rows
      .filter((row) => options.topic === undefined || row.topic === options.topic)
      .filter((row) => options.tenant === undefined || row.tenant === options.tenant)
      .filter((row) => options.subject === undefined || row.subject === options.subject)
      .slice(-limit)
      .reverse();
  }

  since(instant, options = {}) {
    const cutoff = new Date(instant).getTime();
    return this.list({ ...options, limit: this.rows.length }).filter((row) => new Date(row.at).getTime() >= cutoff);
  }

  counts() {
    const out = {};
    for (const row of this.rows) out[row.topic] = (out[row.topic] ?? 0) + 1;
    return out;
  }

  byActor() {
    const out = {};
    for (const row of this.rows) out[row.actor] = (out[row.actor] ?? 0) + 1;
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

  stats() {
    return { events: this.rows.length, topics: Object.keys(this.counts()).length, limit: this.limit };
  }
}

export function eventSummary(log) {
  const counts = log.counts();
  const busiest = Object.entries(counts).sort((left, right) => right[1] - left[1])[0];
  return { events: log.size(), busiest: busiest ?? null };
}

export function topicRows(log, tenant) {
  return Object.entries(log.counts())
    .map(([topic, events]) => ({ topic, events, tenant }))
    .sort((left, right) => right.events - left.events);
}

export function eventsBySubject(log, subject) {
  return log.list({ subject, limit: log.size() });
}

export function recentTopics(log, limit = 10) {
  const rows = log.list({ limit });
  return rows.map((row) => ({ at: row.at, topic: row.topic, actor: row.actor }));
}

export function actorReport(log) {
  return Object.entries(log.byActor())
    .map(([actor, events]) => ({ actor, events }))
    .sort((left, right) => right.events - left.events);
}

export function eventsBetween(log, from, to, tenant) {
  const rows = log.since(from, { tenant });
  return rows.filter((row) => new Date(row.at).getTime() <= new Date(to).getTime());
}

export function isEmptyLog(log) {
  return log.size() === 0;
}
