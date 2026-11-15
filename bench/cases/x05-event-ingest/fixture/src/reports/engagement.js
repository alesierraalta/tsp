import { dayBucket, toInstant } from '../util/time.js';
import { groupBy, sumBy } from '../util/collections.js';
import { percentOf } from '../util/numbers.js';
import { enrichAll } from './enrich.js';

export function trafficMix(events) {
  const enriched = enrichAll(events);
  const groups = groupBy(enriched, (event) => event.derived.device);
  const total = enriched.length;
  return [...groups.entries()]
    .map(([device, rows]) => ({ device, events: rows.length, percent: percentOf(rows.length, total) }))
    .sort((left, right) => right.events - left.events);
}

export function pagePerformance(events) {
  const groups = groupBy(events ?? [], (event) => event.context?.path ?? '(none)');
  return [...groups.entries()]
    .map(([path, rows]) => {
      const durations = rows
        .map((row) => Number(row.properties?.duration_ms))
        .filter((value) => Number.isFinite(value) && value >= 0)
        .sort((left, right) => left - right);
      const p95 = durations.length === 0 ? 0 : durations[Math.min(durations.length - 1, Math.floor(durations.length * 0.95))];
      return { path, views: rows.length, p95 };
    })
    .sort((left, right) => right.views - left.views);
}

export function slowestPaths(events, limit = 5) {
  return pagePerformance(events)
    .filter((row) => row.p95 > 0)
    .sort((left, right) => right.p95 - left.p95)
    .slice(0, limit);
}

export function engagementByDay(events) {
  const groups = groupBy(events ?? [], (event) => dayBucket(event.at));
  return [...groups.entries()]
    .map(([day, rows]) => ({
      day,
      events: rows.length,
      samples: sumBy(rows, (row) => (Array.isArray(row.samples) ? row.samples.length : 0)),
      pages: new Set(rows.map((row) => row.context?.path ?? '(none)')).size,
    }))
    .sort((left, right) => (left.day < right.day ? -1 : 1));
}

export function revenueFromEvents(events) {
  const purchases = (events ?? []).filter((event) => event.name === 'purchase');
  const cents = purchases.reduce((total, event) => total + Number(event.properties?.amount_cents ?? 0), 0);
  return { purchases: purchases.length, cents, average: purchases.length === 0 ? 0 : Math.round(cents / purchases.length) };
}

export function firstTouchOfDay(events) {
  const sorted = [...(events ?? [])].sort((left, right) => (left.at < right.at ? -1 : 1));
  return sorted[0] ? { at: sorted[0].at, name: sorted[0].name } : undefined;
}

export function activeHours(events) {
  const counts = new Array(24).fill(0);
  for (const event of events ?? []) {
    const hour = toInstant(event.at)?.getUTCHours();
    if (hour === undefined) continue;
    counts[hour] += 1;
  }
  return counts.map((events, hour) => ({ hour, events })).sort((left, right) => right.events - left.events);
}

export function engagementReport(events) {
  const mix = trafficMix(events);
  const pages = pagePerformance(events);
  return {
    events: (events ?? []).length,
    mix,
    pages: pages.slice(0, 5),
    slowest: slowestPaths(events, 3),
    revenue_events: revenueFromEvents(events).length,
    active_hours: activeHours(events).length,
  };
}

export function dailyEngagement(events, limit = 14) {
  return engagementByDay(events).slice(-limit);
}

export function bounceByPath(events) {
  const byPath = groupBy(events ?? [], (event) => event.context?.path ?? '(none)');
  const out = [];
  for (const [path, rows] of byPath.entries()) {
    const sessions = new Set(rows.map((row) => row.context?.session ?? row.id));
    const single = [...sessions].filter((session) => rows.filter((row) => (row.context?.session ?? row.id) === session).length === 1);
    out.push({ path, sessions: sessions.size, single: single.length, percent: percentOf(single.length, sessions.size) });
  }
  return out.sort((left, right) => right.sessions - left.sessions);
}

export function humanMinutes(events) {
  const human = enrichAll(events).filter((event) => event.derived.bot === false);
  const total = human.reduce((sum, event) => sum + (Number(event.properties?.duration_ms ?? 0) || 0), 0);
  return Math.round(total / 60000);
}

export function engagementScore(events) {
  const eventsCount = (events ?? []).length;
  const users = new Set((events ?? []).map((event) => event.context?.user ?? event.id)).size;
  const minutes = humanMinutes(events);
  return {
    events: eventsCount,
    users,
    minutes,
    per_user: safeRatio(minutes, users),
    per_event: safeRatio(minutes, eventsCount),
  };
}

function safeRatio(left, right) {
  if (!right) return 0;
  return Math.round((left / right) * 100) / 100;
}
