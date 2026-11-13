import { bucketOf, rangeOf, toInstant } from '../util/time.js';
import { groupBy, sumBy } from '../util/collections.js';

export function rollup(events, { size = 'hour', metrics = ['count'] } = {}) {
  const groups = groupBy(events ?? [], (event) => bucketOf(event.at, size));
  const out = [];
  for (const [bucket, rows] of groups.entries()) {
    const entry = { bucket, events: rows.length };
    if (metrics.includes('samples')) {
      entry.samples = sumBy(rows, (row) => (Array.isArray(row.samples) ? row.samples.length : 0));
    }
    if (metrics.includes('value')) {
      entry.value = rows.reduce((total, row) => total + (Array.isArray(row.samples) ? row.samples.reduce((a, b) => a + b, 0) : 0), 0);
    }
    out.push(entry);
  }
  return out.sort((left, right) => (left.bucket < right.bucket ? -1 : 1));
}

export function rollupByEvent(events, size = 'hour') {
  const out = new Map();
  for (const event of events ?? []) {
    const key = `${bucketOf(event.at, size)}|${event.name}`;
    out.set(key, (out.get(key) ?? 0) + 1);
  }
  return [...out.entries()]
    .map(([key, count]) => {
      const [bucket, name] = key.split('|');
      return { bucket, name, count };
    })
    .sort((left, right) => (left.bucket === right.bucket ? (left.name < right.name ? -1 : 1) : left.bucket < right.bucket ? -1 : 1));
}

export function densify(points, from, to, size = 'hour') {
  const byBucket = new Map(points.map((point) => [point.bucket, point]));
  return rangeOf(from, to, size).map((bucket) => byBucket.get(bucket) ?? { bucket, events: 0 });
}

export function growthOf(points) {
  if (points.length < 2) return 0;
  const first = points[0].events;
  const last = points[points.length - 1].events;
  if (first === 0) return last === 0 ? 0 : 100;
  return Math.round(((last - first) / first) * 10000) / 100;
}

export function peakOf(points) {
  return points.reduce((best, point) => (best === undefined || point.events > best.events ? point : best), undefined);
}

export function rollupTotals(points) {
  return {
    points: points.length,
    events: points.reduce((total, point) => total + (point.events ?? 0), 0),
    first: points[0]?.bucket,
    last: points[points.length - 1]?.bucket,
  };
}

export function spanHours(points) {
  if (points.length === 0) return 0;
  const first = toInstant(points[0].bucket).getTime();
  const last = toInstant(points[points.length - 1].bucket).getTime();
  return Math.round((last - first) / 3600000);
}
