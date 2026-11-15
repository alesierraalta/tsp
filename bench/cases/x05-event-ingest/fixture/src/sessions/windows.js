import { bucketOf, toInstant } from '../util/time.js';

export const WINDOW_SIZES = ['minute', 'hour', 'day'];

export function normalizeWindow(input = {}) {
  const size = WINDOW_SIZES.includes(input.size) ? input.size : 'hour';
  const count = Math.max(1, Math.trunc(Number(input.count ?? 1)));
  return { size, count };
}

export function windowStart(value, window = { size: 'hour', count: 1 }) {
  return bucketOf(value, window.size);
}

export function windowEnd(start, window = { size: 'hour', count: 1 }) {
  const instant = toInstant(start);
  if (!instant) return undefined;
  const unit = window.size === 'minute' ? 60000 : window.size === 'day' ? 86400000 : 3600000;
  return new Date(instant.getTime() + unit * window.count).toISOString();
}

export function inWindow(value, start, window) {
  const instant = toInstant(value)?.getTime();
  const from = toInstant(start)?.getTime();
  const to = toInstant(windowEnd(start, window))?.getTime();
  if (instant === undefined || from === undefined || to === undefined) return false;
  return instant >= from && instant < to;
}

export function groupByWindow(events, window = { size: 'hour', count: 1 }) {
  const out = new Map();
  for (const event of events ?? []) {
    const start = windowStart(event.at, window);
    if (!start) continue;
    const bucket = out.get(start);
    if (bucket) bucket.push(event);
    else out.set(start, [event]);
  }
  return [...out.entries()].sort((left, right) => (left[0] < right[0] ? -1 : 1)).map(([start, rows]) => ({ start, rows }));
}

export function windowSummary(events, window) {
  const groups = groupByWindow(events, window);
  return {
    windows: groups.length,
    busiest: groups.reduce((best, group) => (best === undefined || group.rows.length > best.rows.length ? group : best), undefined),
    empty: groups.filter((group) => group.rows.length === 0).length,
  };
}

export function fillWindows(events, window, from, to) {
  const groups = new Map(groupByWindow(events, window).map((group) => [group.start, group.rows.length]));
  const out = [];
  let cursor = windowStart(from, window);
  const limit = toInstant(to)?.getTime() ?? 0;
  const unit = window.size === 'minute' ? 60000 : window.size === 'day' ? 86400000 : 3600000;
  while (cursor && toInstant(cursor).getTime() <= limit) {
    out.push({ start: cursor, count: groups.get(cursor) ?? 0 });
    cursor = new Date(toInstant(cursor).getTime() + unit).toISOString();
  }
  return out;
}

export function windowLabels(events, window) {
  return groupByWindow(events, window).map((bucket) => bucket.bucket ?? bucket.start);
}

export function overlappingWindows(events, windows) {
  const out = [];
  for (const window of windows ?? []) {
    const rows = (events ?? []).filter((event) => inWindow(event.at, windowStart(event.at, window), window));
    out.push({ window: normalizeWindow(window), events: rows.length });
  }
  return out;
}

export function sparseWindows(events, window, minimum = 1) {
  const summary = windowSummary(events, window);
  return (summary.buckets ?? []).filter((bucket) => (bucket.events ?? bucket.count ?? 0) < minimum);
}

export function densestWindow(events, window) {
  const summary = windowSummary(events, window);
  return (summary.buckets ?? []).reduce((best, bucket) => {
    const value = bucket.events ?? bucket.count ?? 0;
    const bestValue = best === undefined ? -1 : (best.events ?? best.count ?? 0);
    return value > bestValue ? bucket : best;
  }, undefined);
}

export function windowSpan(window) {
  const normalized = normalizeWindow(window);
  const unit = normalized.size === 'minute' ? 60000 : normalized.size === 'hour' ? 3600000 : 86400000;
  return { size: normalized.size, count: normalized.count, milliseconds: unit * normalized.count };
}
