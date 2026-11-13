import { quantile } from '../util/numbers.js';

export function histogram(values, buckets = 10) {
  const list = (values ?? []).map(Number).filter((value) => Number.isFinite(value));
  if (list.length === 0) return { buckets: [], min: 0, max: 0, count: 0 };
  const min = Math.min(...list);
  const max = Math.max(...list);
  const width = (max - min) / buckets || 1;
  const counts = new Array(buckets).fill(0);
  for (const value of list) {
    const index = Math.min(buckets - 1, Math.floor((value - min) / width));
    counts[index] += 1;
  }
  return {
    buckets: counts.map((count, index) => ({ from: min + index * width, to: min + (index + 1) * width, count })),
    min,
    max,
    count: list.length,
  };
}

export function percentileSummary(values) {
  const list = (values ?? []).map(Number).filter((value) => Number.isFinite(value));
  return {
    count: list.length,
    p50: quantile(list, 0.5),
    p90: quantile(list, 0.9),
    p99: quantile(list, 0.99),
    max: list.length === 0 ? 0 : Math.max(...list),
  };
}

export function reservoir(values, size = 20, random = Math.random) {
  const out = [];
  let seen = 0;
  for (const value of values ?? []) {
    seen += 1;
    if (out.length < size) {
      out.push(value);
      continue;
    }
    const index = Math.floor(random() * seen);
    if (index < size) out[index] = value;
  }
  return out;
}

export function sampleValues(events) {
  return (events ?? []).flatMap((event) => (Array.isArray(event.samples) ? event.samples : []));
}

export function sampleTotals(events) {
  const values = sampleValues(events);
  return {
    values: values.length,
    sum: values.reduce((total, value) => total + value, 0),
    average: values.length === 0 ? 0 : values.reduce((total, value) => total + value, 0) / values.length,
  };
}

export function truncateSamples(event, limit) {
  if (!Array.isArray(event.samples)) return event;
  return { ...event, samples: event.samples.slice(0, Math.max(0, limit)) };
}

export function sampleFootprint(events) {
  return (events ?? []).reduce((total, event) => total + (Array.isArray(event.samples) ? event.samples.length : 0) * 8, 0);
}

export function sampleSketch(events) {
  const values = sampleValues(events);
  const totals = sampleTotals(events);
  return {
    values: totals.values,
    sum: totals.sum,
    average: Math.round(totals.average * 100) / 100,
    min: values.length === 0 ? 0 : Math.min(...values),
    max: values.length === 0 ? 0 : Math.max(...values),
    distinct: new Set(values).size,
  };
}

export function sampleCaps(events, cap) {
  const limit = Math.max(0, Math.trunc(Number(cap) || 0));
  return (events ?? []).map((event) => ({
    name: event.name,
    kept: Array.isArray(event.samples) ? Math.min(limit, event.samples.length) : 0,
    dropped: Array.isArray(event.samples) ? Math.max(0, event.samples.length - limit) : 0,
  }));
}

export function cappedSamples(events, cap) {
  return (events ?? []).map((event) => truncateSamples(event, cap));
}

export function sampleFootprintReport(events) {
  const bytes = sampleFootprint(events);
  return { events: (events ?? []).length, bytes, kib: Math.round((bytes / 1024) * 10) / 10 };
}
