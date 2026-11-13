import { groupBy, sumBy } from '../util/collections.js';
import { dayBucket, toInstant } from '../util/time.js';
import { percentOf, safeDivide } from '../util/numbers.js';

export function kpis(events, previous = []) {
  const users = new Set((events ?? []).map((event) => event.context?.user ?? event.id)).size;
  const previousUsers = new Set((previous ?? []).map((event) => event.context?.user ?? event.id)).size;
  return {
    events: (events ?? []).length,
    users,
    events_per_user: Math.round(safeDivide((events ?? []).length, users, 0) * 100) / 100,
    growth: percentOf(users - previousUsers, previousUsers || 1),
    samples: sumBy(events ?? [], (event) => (Array.isArray(event.samples) ? event.samples.length : 0)),
  };
}

export function dailySeries(events) {
  const groups = groupBy(events ?? [], (event) => dayBucket(event.at));
  return [...groups.entries()]
    .map(([day, rows]) => ({
      day,
      events: rows.length,
      users: new Set(rows.map((row) => row.context?.user ?? row.id)).size,
    }))
    .sort((left, right) => (left.day < right.day ? -1 : 1));
}

export function weekOverWeek(events) {
  const points = dailySeries(events);
  if (points.length < 14) return { weeks: points.length, change: 0 };
  const last = points.slice(-7).reduce((total, point) => total + point.users, 0);
  const previous = points.slice(-14, -7).reduce((total, point) => total + point.users, 0);
  return { weeks: 2, last, previous, change: percentOf(last - previous, previous || 1) };
}

export function topPaths(events, limit = 5) {
  const counts = new Map();
  for (const event of events ?? []) {
    const path = String(event.context?.path ?? '(none)');
    counts.set(path, (counts.get(path) ?? 0) + 1);
  }
  return [...counts.entries()]
    .map(([path, events]) => ({ path, events }))
    .sort((left, right) => right.events - left.events)
    .slice(0, limit);
}

export function summaryLine(events) {
  const report = kpis(events);
  return `${report.events} events from ${report.users} users (${report.events_per_user} each)`;
}

export function digest(tenant, events, options = {}) {
  return {
    tenant,
    generated_at: new Date(options.now ?? Date.now()).toISOString(),
    kpis: kpis(events, options.previous),
    series: dailySeries(events),
    topPaths: topPaths(events),
    headline: summaryLine(events),
  };
}

export function compareDigests(before, after) {
  const users = after.kpis.users - before.kpis.users;
  const events = after.kpis.events - before.kpis.events;
  return {
    users,
    events,
    direction: users === 0 ? 'flat' : users > 0 ? 'up' : 'down',
    percent: percentOf(users, before.kpis.users || 1),
  };
}

export function periodBounds(events) {
  const times = (events ?? []).map((event) => toInstant(event.at)?.getTime() ?? 0).filter(Boolean);
  if (times.length === 0) return { from: undefined, to: undefined };
  return { from: new Date(Math.min(...times)).toISOString(), to: new Date(Math.max(...times)).toISOString() };
}

export function kpiTable(events, options = {}) {
  const found = kpis(events, options.previous);
  return [
    { name: 'events', value: found.events, unit: 'count' },
    { name: 'users', value: found.users, unit: 'count' },
    { name: 'events_per_user', value: found.events_per_user, unit: 'ratio' },
    { name: 'samples', value: found.samples, unit: 'count' },
  ];
}

export function kpiDeltas(before, after) {
  const deltas = {};
  for (const name of Object.keys(before ?? {})) {
    const left = Number(before[name]) || 0;
    const right = Number(after[name]) || 0;
    deltas[name] = { before: left, after: right, delta: right - left, percent: left === 0 ? (right === 0 ? 0 : 100) : Math.round(((right - left) / left) * 10000) / 100 };
  }
  return deltas;
}

export function kpiTrend(kpis, name, threshold = 5) {
  const row = (kpis ?? []).find((entry) => entry.name === name);
  if (!row) return 'unknown';
  if (Math.abs(row.delta ?? 0) < threshold) return 'flat';
  return (row.delta ?? 0) > 0 ? 'up' : 'down';
}

export function topKpi(kpis) {
  return (kpis ?? []).slice().sort((left, right) => (right.value ?? 0) - (left.value ?? 0))[0];
}

export function kpiSummary(events, options = {}) {
  const found = kpis(events, options.previous);
  const table = kpiTable(events, options);
  return { kpis: found, table, headline: topKpi(table) };
}
