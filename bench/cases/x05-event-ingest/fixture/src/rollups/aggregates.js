import { groupBy, sumBy, topBy } from '../util/collections.js';
import { percentOf, quantile, safeDivide } from '../util/numbers.js';

export function aggregateSamples(events) {
  const values = (events ?? []).flatMap((event) => (Array.isArray(event.samples) ? event.samples : []));
  if (values.length === 0) return { count: 0, sum: 0, mean: 0, p50: 0, p95: 0, max: 0 };
  return {
    count: values.length,
    sum: values.reduce((total, value) => total + value, 0),
    mean: values.reduce((total, value) => total + value, 0) / values.length,
    p50: quantile(values, 0.5),
    p95: quantile(values, 0.95),
    max: Math.max(...values),
  };
}

export function aggregateByEvent(events) {
  const groups = groupBy(events ?? [], (event) => event.name);
  return [...groups.entries()]
    .map(([name, rows]) => ({ name, ...aggregateSamples(rows), events: rows.length }))
    .sort((left, right) => right.events - left.events);
}

export function conversionRate(events, from, to) {
  const starts = (events ?? []).filter((event) => event.name === from).length;
  const finishes = (events ?? []).filter((event) => event.name === to).length;
  return { starts, finishes, percent: percentOf(finishes, starts) };
}

export function retentionMatrix(events, days = 7) {
  const first = new Map();
  for (const event of events ?? []) {
    const user = event.context?.user ?? event.id;
    const day = String(event.at ?? '').slice(0, 10);
    if (!first.has(user) || day < first.get(user)) first.set(user, day);
  }
  const matrix = [];
  for (let offset = 0; offset < days; offset += 1) {
    matrix.push({
      offset,
      retained: (events ?? []).filter((event) => {
        const user = event.context?.user ?? event.id;
        const start = new Date(`${first.get(user)}T00:00:00.000Z`).getTime();
        const at = new Date(event.at).getTime();
        return Math.floor((at - start) / 86400000) === offset;
      }).length,
    });
  }
  return matrix;
}

export function busiestHour(events) {
  const groups = groupBy(events ?? [], (event) => String(event.at ?? '').slice(11, 13));
  const ranked = topBy([...groups.entries()].map(([hour, rows]) => ({ hour, events: rows.length })), (row) => row.events, 1);
  return ranked[0];
}

export function dailyAverage(events, days) {
  return safeDivide((events ?? []).length, days, 0);
}

export function samplesPerEvent(events) {
  const total = sumBy(events ?? [], (event) => (Array.isArray(event.samples) ? event.samples.length : 0));
  return safeDivide(total, (events ?? []).length, 0);
}

export function aggregateReport(events) {
  return {
    events: (events ?? []).length,
    samples: aggregateSamples(events),
    byEvent: aggregateByEvent(events),
    busiestHour: busiestHour(events),
    samplesPerEvent: samplesPerEvent(events),
  };
}

export function sampleBuckets(events, edges = []) {
  const values = (events ?? []).flatMap((event) => (Array.isArray(event.samples) ? event.samples : [])).map(Number);
  const sorted = edges.slice().sort((left, right) => left - right);
  const out = sorted.map((edge) => ({ edge, count: values.filter((value) => value <= edge).length }));
  return { buckets: out, values: values.length };
}

export function sampleQuantiles(events, fractions = [0.5, 0.9, 0.99]) {
  const values = (events ?? []).flatMap((event) => (Array.isArray(event.samples) ? event.samples : [])).map(Number);
  return (fractions ?? []).map((fraction) => ({ fraction, value: quantile(values, fraction) }));
}

export function perEventSamples(events) {
  return (events ?? []).map((event) => ({
    name: event.name,
    samples: Array.isArray(event.samples) ? event.samples.length : 0,
    mean: Array.isArray(event.samples) && event.samples.length > 0 ? event.samples.reduce((a, b) => a + Number(b), 0) / event.samples.length : 0,
  }));
}

export function aggregateTable(events) {
  return aggregateByEvent(events).map((row) => ({
    name: row.name,
    events: row.events,
    samples: row.samples,
    average: row.events === 0 ? 0 : Math.round((row.samples / row.events) * 100) / 100,
    share: percentOf(row.samples, aggregateSamples(events).sum),
  }));
}

export function sampleOutliers(events, factor = 3) {
  const values = (events ?? []).flatMap((event) => (Array.isArray(event.samples) ? event.samples : [])).map(Number);
  if (values.length === 0) return [];
  const mean = values.reduce((total, value) => total + value, 0) / values.length;
  const cut = mean * Math.max(1, factor);
  return values.filter((value) => value > cut).sort((left, right) => right - left);
}
