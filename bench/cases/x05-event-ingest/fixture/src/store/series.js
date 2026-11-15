import { bucketOf, toInstant } from '../util/time.js';

export class SeriesStore {
  constructor(options = {}) {
    this.clock = options.clock ?? (() => new Date());
    this.series = new Map();
  }

  keyOf(tenant, metric, dimension) {
    return `${tenant}|${metric}|${dimension ?? ''}`;
  }

  add(tenant, metric, at, value, dimension) {
    const key = this.keyOf(tenant, metric, dimension);
    const bucket = bucketOf(at, 'hour');
    let points = this.series.get(key);
    if (!points) {
      points = new Map();
      this.series.set(key, points);
    }
    const current = points.get(bucket) ?? { bucket, count: 0, total: 0, min: undefined, max: undefined };
    current.count += 1;
    current.total += Number(value) || 0;
    current.min = current.min === undefined ? value : Math.min(current.min, value);
    current.max = current.max === undefined ? value : Math.max(current.max, value);
    points.set(bucket, current);
    return current;
  }

  points(tenant, metric, dimension) {
    return [...(this.series.get(this.keyOf(tenant, metric, dimension))?.values() ?? [])].sort((left, right) =>
      left.bucket < right.bucket ? -1 : 1,
    );
  }

  between(tenant, metric, from, to, dimension) {
    const start = toInstant(from)?.getTime() ?? 0;
    const end = toInstant(to)?.getTime() ?? Number.MAX_SAFE_INTEGER;
    return this.points(tenant, metric, dimension).filter((point) => {
      const time = toInstant(point.bucket).getTime();
      return time >= start && time <= end;
    });
  }

  total(tenant, metric, dimension) {
    return this.points(tenant, metric, dimension).reduce((total, point) => total + point.total, 0);
  }

  keys() {
    return [...this.series.keys()];
  }

  dimensions(tenant, metric) {
    return this.keys()
      .filter((key) => key.startsWith(`${tenant}|${metric}|`))
      .map((key) => key.split('|')[2])
      .filter(Boolean);
  }

  prune(before) {
    const cutoff = toInstant(before)?.getTime() ?? 0;
    let removed = 0;
    for (const [key, points] of this.series.entries()) {
      for (const bucket of [...points.keys()]) {
        if ((toInstant(bucket)?.getTime() ?? 0) < cutoff) {
          points.delete(bucket);
          removed += 1;
        }
      }
      if (points.size === 0) this.series.delete(key);
    }
    return removed;
  }

  stats() {
    let points = 0;
    for (const bucket of this.series.values()) points += bucket.size;
    return { series: this.series.size, points };
  }

  // Compaction folds the hour points of one series into day points: the sweeper applies it to a series the
  // plan keeps for longer than the raw events, so the counts survive the raw rows the retention window drops.
  compact(tenant, metric, dimension) {
    const key = this.keyOf(tenant, metric, dimension);
    const points = this.series.get(key);
    if (!points) return { before: 0, after: 0 };
    const byDay = new Map();
    for (const point of points.values()) {
      const day = String(point.bucket).slice(0, 10);
      const current = byDay.get(day) ?? { bucket: `${day}T00:00:00.000Z`, count: 0, total: 0 };
      current.count += point.count;
      current.total += point.total;
      current.min = current.min === undefined ? point.min : Math.min(current.min, point.min ?? point.max);
      current.max = current.max === undefined ? point.max : Math.max(current.max, point.max ?? point.min);
      byDay.set(day, current);
    }
    this.series.set(key, new Map([...byDay.values()].map((point) => [point.bucket, point])));
    return { before: points.size, after: byDay.size };
  }

  compactAll(tenant) {
    const metrics = [...new Set(this.keys().filter((key) => key.startsWith(`${tenant}|`)).map((key) => key.split('|')[1]))];
    return metrics.map((metric) => ({ metric, ...this.compact(tenant, metric) }));
  }

  trim(tenant, metric, keep = 24) {
    const points = this.series.get(this.keyOf(tenant, metric));
    if (!points) return 0;
    const sorted = [...points.values()].sort((left, right) => (left.bucket < right.bucket ? -1 : 1));
    const excess = Math.max(0, sorted.length - Math.max(1, Math.trunc(keep)));
    for (const point of sorted.slice(0, excess)) points.delete(point.bucket);
    return excess;
  }
}

export function rollupPoints(points, size = 3) {
  const out = [];
  for (let index = 0; index < points.length; index += size) {
    const slice = points.slice(index, index + size);
    if (slice.length === 0) continue;
    out.push({
      bucket: slice[0].bucket,
      count: slice.reduce((total, point) => total + point.count, 0),
      total: slice.reduce((total, point) => total + point.total, 0),
    });
  }
  return out;
}
