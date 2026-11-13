import { dayBucket, toInstant } from '../util/time.js';
import { countBy, groupBy, uniqueBy } from '../util/collections.js';
import { percentOf } from '../util/numbers.js';

export function cohortKey(event) {
  const user = event.context?.user ?? event.context?.session ?? event.id;
  const day = dayBucket(event.at);
  return `${user}|${day}`;
}

export function firstSeen(events) {
  const out = new Map();
  for (const event of events ?? []) {
    const user = event.context?.user ?? event.id;
    const day = dayBucket(event.at);
    const current = out.get(user);
    if (current === undefined || day < current) out.set(user, day);
  }
  return out;
}

export function cohortTable(events) {
  const first = firstSeen(events);
  const table = new Map();
  for (const [user, day] of first.entries()) {
    const bucket = table.get(day) ?? { day, users: 0, events: 0 };
    bucket.users += 1;
    bucket.events += (events ?? []).filter((event) => (event.context?.user ?? event.id) === user).length;
    table.set(day, bucket);
  }
  return [...table.values()].sort((left, right) => (left.day < right.day ? -1 : 1));
}

export function cohortRetention(events, days = 7) {
  const first = firstSeen(events);
  const out = [];
  for (let offset = 0; offset < days; offset += 1) {
    const returning = (events ?? []).filter((event) => {
      const user = event.context?.user ?? event.id;
      const start = toInstant(first.get(user));
      const at = toInstant(event.at);
      if (!start || !at) return false;
      const dayOffset = Math.floor((at.getTime() - start.getTime()) / 86400000);
      return dayOffset === offset;
    });
    out.push({ offset, users: uniqueBy(returning, (event) => event.context?.user ?? event.id).length });
  }
  return out;
}

export function retentionCurve(from, to, totalUsers) {
  return from.map((point, index) => ({
    offset: point.offset,
    users: point.users,
    percent: percentOf(point.users, totalUsers || to[index]?.users || 1),
  }));
}

export function cohortSize(events) {
  return uniqueBy(events ?? [], (event) => event.context?.user ?? event.id).length;
}

export function dailyActives(events) {
  const groups = groupBy(events ?? [], (event) => dayBucket(event.at));
  return [...groups.entries()]
    .map(([day, rows]) => ({ day, users: uniqueBy(rows, (row) => row.context?.user ?? row.id).length, events: rows.length }))
    .sort((left, right) => (left.day < right.day ? -1 : 1));
}

export function eventsPerUser(events) {
  const counts = countBy(events ?? [], (event) => event.context?.user ?? event.id);
  const values = [...counts.values()];
  return values.length === 0 ? 0 : Math.round((values.reduce((total, value) => total + value, 0) / values.length) * 100) / 100;
}

export function recurrence(events, days = 7) {
  const first = firstSeen(events);
  const byUser = groupBy(events ?? [], (event) => event.context?.user ?? event.id);
  const rows = [];
  for (const [user, day] of first.entries()) {
    const active = new Set((byUser.get(user) ?? []).map((event) => dayBucket(event.at)));
    const span = [...active].sort();
    const window = [];
    for (let index = 0; index < days; index += 1) window.push(active.has(addDaysFrom(day, index)) ? 1 : 0);
    rows.push({ user, day, first_active: span[0] ?? day, last_active: span[span.length - 1] ?? day, days_active: active.size, window });
  }
  return rows;
}

function addDaysFrom(day, days) {
  const base = new Date(`${day}T00:00:00.000Z`);
  return new Date(base.getTime() + days * 86400000).toISOString().slice(0, 10);
}

export function recurrenceCurve(events, days = 7) {
  const rows = recurrence(events, days);
  const totals = new Array(days).fill(0);
  for (const row of rows) row.window.forEach((active, index) => (totals[index] += active));
  return totals.map((active, index) => ({
    day: index,
    users: active,
    percent: rows.length === 0 ? 0 : percentOf(active, rows.length),
  }));
}

export function cohortReport(events, days = 7) {
  const rows = recurrence(events, days);
  return {
    users: rows.length,
    cohorts: new Set(rows.map((row) => row.day)).size,
    returning: rows.filter((row) => row.days_active > 1).length,
    curve: recurrenceCurve(events, days),
  };
}
