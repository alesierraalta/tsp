import { channelOf } from '../sessions/attribution.js';
import { groupBy, sumBy } from '../util/collections.js';
import { percentOf } from '../util/numbers.js';
import { dayBucket } from '../util/time.js';

export function dashboard(tenant, events, options = {}) {
  const days = groupBy(events ?? [], (event) => dayBucket(event.at));
  const points = [...days.entries()]
    .map(([day, rows]) => ({
      day,
      events: rows.length,
      users: new Set(rows.map((row) => row.context?.user ?? row.id)).size,
      samples: sumBy(rows, (row) => (Array.isArray(row.samples) ? row.samples.length : 0)),
    }))
    .sort((left, right) => (left.day < right.day ? -1 : 1));
  const totals = {
    events: events?.length ?? 0,
    users: new Set((events ?? []).map((event) => event.context?.user ?? event.id)).size,
    samples: sumBy(events ?? [], (event) => (Array.isArray(event.samples) ? event.samples.length : 0)),
  };
  return {
    tenant,
    days: points.length,
    points,
    totals,
    average_events: points.length === 0 ? 0 : Math.round((totals.events / points.length) * 100) / 100,
    title: options.title ?? 'Event volume',
  };
}

export function channelPanel(events) {
  const counts = {};
  for (const event of events ?? []) {
    const channel = channelOf(event);
    counts[channel] = (counts[channel] ?? 0) + 1;
  }
  const total = Object.values(counts).reduce((sum, value) => sum + value, 0);
  return Object.entries(counts)
    .map(([channel, events]) => ({ channel, events, percent: percentOf(events, total) }))
    .sort((left, right) => right.events - left.events);
}

export function funnelPanel(events, steps = ['page_view', 'purchase']) {
  const reached = new Map();
  for (const event of events ?? []) {
    const index = steps.indexOf(event.name);
    if (index < 0) continue;
    const key = event.context?.session ?? event.context?.user ?? event.id;
    const current = reached.get(key) ?? 0;
    if (index === current) reached.set(key, current + 1);
  }
  return steps.map((step, index) => ({
    step,
    users: [...reached.values()].filter((value) => value > index).length,
  }));
}

export function anomalyPanel(points, factor = 2) {
  if (points.length < 3) return [];
  const values = points.map((point) => point.events);
  const mean = values.reduce((total, value) => total + value, 0) / values.length;
  return points
    .map((point) => ({ day: point.day, events: point.events, ratio: mean === 0 ? 0 : Math.round((point.events / mean) * 100) / 100 }))
    .filter((point) => point.ratio >= factor);
}

export function panelSummary(dash) {
  return { tenant: dash.tenant, days: dash.days, events: dash.totals.events, users: dash.totals.users };
}

export const PANELS = ['volume', 'channels', 'funnel', 'anomaly', 'cohorts'];

export function panel(events, kind, options = {}) {
  switch (kind) {
    case 'volume': {
      const days = groupBy(events ?? [], (event) => dayBucket(event.at));
      return { kind, points: [...days.keys()].sort().map((day) => ({ day, events: days.get(day).length })) };
    }
    case 'channels':
      return { kind, channels: channelPanel(events) };
    case 'funnel':
      return { kind, funnel: funnelPanel(events, options.steps) };
    case 'anomaly':
      return { kind, anomalies: anomalyPanel(dashboard('panel', events).points, options.factor ?? 2) };
    case 'cohorts':
      return { kind, users: new Set((events ?? []).map((event) => event.context?.user ?? event.id)).size };
    default:
      throw new TypeError(`unknown panel ${kind}`);
  }
}

export function panels(events, kinds = PANELS, options = {}) {
  return kinds.map((kind) => panel(events, kind, options));
}

export function panelNames() {
  return [...PANELS];
}

export function dashboardHeader(tenant, dash, options = {}) {
  return {
    tenant,
    title: options.title ?? `${tenant} activity`,
    days: dash.days,
    events: dash.totals.events,
    users: dash.totals.users,
    generated_at: options.now ?? new Date().toISOString(),
  };
}

export function dashboardSections(dash) {
  return [
    { section: 'activity', rows: dash.points.length },
    { section: 'channels', rows: Object.keys(channelPanel(dash.points)).length },
    { section: 'funnel', rows: funnelPanel(dash.points).length },
  ];
}
