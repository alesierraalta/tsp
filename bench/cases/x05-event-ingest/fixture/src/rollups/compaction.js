import { toInstant } from '../util/time.js';
import { rollup } from './rollup.js';

export function compacted(points, size) {
  const step = Math.max(1, Math.trunc(size));
  const out = [];
  for (let index = 0; index < points.length; index += step) {
    const slice = points.slice(index, index + step);
    if (slice.length === 0) continue;
    out.push({
      bucket: slice[0].bucket,
      events: slice.reduce((total, point) => total + (point.events ?? 0), 0),
      samples: slice.reduce((total, point) => total + (point.samples ?? 0), 0),
      points: slice.length,
    });
  }
  return out;
}

export function compactedRollup(events, size, step) {
  return compacted(rollup(events, { size, metrics: ['samples'] }), step);
}

export function dropEmpty(points) {
  return (points ?? []).filter((point) => (point.events ?? 0) > 0);
}

export function trimBefore(points, instant) {
  const cutoff = toInstant(instant)?.getTime() ?? 0;
  return (points ?? []).filter((point) => (toInstant(point.bucket)?.getTime() ?? 0) >= cutoff);
}

export function compactPlan(sizes = ['hour', 'day', 'month']) {
  return sizes.map((size, index) => ({
    size,
    keep: [24, 90, 730][index] ?? 365,
    from: index === 0 ? 'hour' : sizes[index - 1],
  }));
}

export function retentionCutoff(size, clock = () => new Date()) {
  const days = size === 'hour' ? 7 : size === 'day' ? 90 : 730;
  return new Date(new Date(clock()).getTime() - days * 86400000).toISOString();
}

export function compactionReport(points, compactedPoints) {
  return {
    input: points.length,
    output: compactedPoints.length,
    saved: points.length - compactedPoints.length,
    events: compactedPoints.reduce((total, point) => total + (point.events ?? 0), 0),
  };
}

export function mergeCompacted(left, right) {
  const byBucket = new Map((left ?? []).map((point) => [point.bucket, { ...point }]));
  for (const point of right ?? []) {
    const existing = byBucket.get(point.bucket);
    if (existing) {
      existing.events += point.events ?? 0;
      existing.samples = (existing.samples ?? 0) + (point.samples ?? 0);
      continue;
    }
    byBucket.set(point.bucket, { ...point });
  }
  return [...byBucket.values()].sort((a, b) => (a.bucket < b.bucket ? -1 : 1));
}
