import { minuteKey } from '../util/time.js';

export function groupBy(rows, keyOf) {
  const groups = new Map();
  for (const row of rows) {
    const key = String(keyOf(row) ?? '');
    if (!groups.has(key)) groups.set(key, []);
    groups.get(key).push(row);
  }
  return groups;
}

export function statusCounts(rows) {
  const groups = groupBy(rows, (row) => row.status);
  const out = {};
  for (const [status, entries] of groups.entries()) out[status] = entries.length;
  return out;
}

export function hostCounts(rows) {
  const groups = groupBy(rows, (row) => row.target ?? row.host);
  return [...groups.entries()]
    .map(([host, entries]) => ({ host, rows: entries.length }))
    .sort((left, right) => right.rows - left.rows || (left.host < right.host ? -1 : 1));
}

export function topicCounts(rows) {
  const groups = groupBy(rows, (row) => row.topic ?? 'default');
  return [...groups.entries()]
    .map(([topic, entries]) => ({ topic, rows: entries.length }))
    .sort((left, right) => (left.topic < right.topic ? -1 : 1));
}

export function successRate(counts) {
  const delivered = counts.delivered ?? 0;
  const attempted = delivered + (counts.failed ?? 0);
  if (attempted === 0) return 0;
  return Math.round((delivered / attempted) * 1000) / 10;
}

export function attemptHistogram(rows) {
  const buckets = {};
  for (const row of rows) {
    const attempts = Number(row.attempts ?? 0);
    buckets[attempts] = (buckets[attempts] ?? 0) + 1;
  }
  return Object.fromEntries(Object.entries(buckets).sort((left, right) => Number(left[0]) - Number(right[0])));
}

export function errorCounts(rows) {
  const counts = {};
  for (const row of rows) {
    const message = row.last_error?.code ?? row.last_error?.message;
    if (!message) continue;
    counts[message] = (counts[message] ?? 0) + 1;
  }
  return counts;
}

export function topErrors(rows, limit = 5) {
  return Object.entries(errorCounts(rows))
    .map(([error, count]) => ({ error, count }))
    .sort((left, right) => right.count - left.count || (left.error < right.error ? -1 : 1))
    .slice(0, limit);
}

export function timeline(rows, options = {}) {
  const buckets = new Map();
  for (const row of rows) {
    const key = minuteKey(row.created_at ?? new Date().toISOString());
    const bucket = buckets.get(key) ?? { minute: key, total: 0, delivered: 0, failed: 0 };
    bucket.total += 1;
    if (row.status === 'delivered') bucket.delivered += 1;
    if (row.status === 'failed') bucket.failed += 1;
    buckets.set(key, bucket);
  }
  return [...buckets.values()]
    .sort((left, right) => (left.minute < right.minute ? -1 : 1))
    .slice(-(options.limit ?? 30));
}

export function percentile(values, fraction) {
  if (!Array.isArray(values) || values.length === 0) return 0;
  const sorted = [...values].sort((left, right) => left - right);
  const position = Math.min(sorted.length - 1, Math.max(0, Math.ceil(fraction * sorted.length) - 1));
  return sorted[position];
}

export function attemptStats(rows) {
  const attempts = rows.map((row) => Number(row.attempts ?? 0));
  return {
    total: attempts.reduce((sum, value) => sum + value, 0),
    max: attempts.length === 0 ? 0 : Math.max(...attempts),
    p50: percentile(attempts, 0.5),
    p95: percentile(attempts, 0.95),
  };
}

export function deliveryDigest(rows, options = {}) {
  const counts = statusCounts(rows);
  return {
    total: rows.length,
    statuses: counts,
    success_rate: successRate(counts),
    hosts: hostCounts(rows).slice(0, options.hosts ?? 10),
    topics: topicCounts(rows),
    attempts: attemptStats(rows),
    errors: topErrors(rows, options.errors ?? 5),
    seen_at: new Date(options.now ?? Date.now()).toISOString(),
  };
}

export function compareDigests(before, after) {
  return {
    total: (after.total ?? 0) - (before.total ?? 0),
    delivered: (after.statuses?.delivered ?? 0) - (before.statuses?.delivered ?? 0),
    failed: (after.statuses?.failed ?? 0) - (before.statuses?.failed ?? 0),
    success_rate: Math.round(((after.success_rate ?? 0) - (before.success_rate ?? 0)) * 10) / 10,
    changed:
      (after.total ?? 0) !== (before.total ?? 0) || (after.success_rate ?? 0) !== (before.success_rate ?? 0),
  };
}

export function digestLines(digest) {
  const lines = [
    `deliveries: ${digest.total}`,
    `success: ${digest.success_rate}%`,
    `attempts: max=${digest.attempts.max} p50=${digest.attempts.p50} p95=${digest.attempts.p95}`,
  ];
  for (const status of Object.keys(digest.statuses).sort()) {
    lines.push(`status ${status}: ${digest.statuses[status]}`);
  }
  for (const host of digest.hosts) lines.push(`host ${host.host}: ${host.rows}`);
  for (const topic of digest.topics) lines.push(`topic ${topic.topic}: ${topic.rows}`);
  for (const error of digest.errors) lines.push(`error ${error.error}: ${error.count}`);
  return lines;
}

export function renderDigest(digest) {
  return digestLines(digest).join('\n');
}

export function digestSummary(digest) {
  return `${digest.total} deliveries ${digest.success_rate}% delivered`;
}

export function staleness(rows, options = {}) {
  const now = new Date(options.now ?? Date.now()).getTime();
  const limit = options.staleSeconds ?? 300;
  return rows
    .filter((row) => row.status === 'pending' || row.status === 'retrying')
    .map((row) => ({
      id: row.id,
      age_seconds: Math.max(0, Math.round((now - new Date(row.updated_at ?? row.created_at).getTime()) / 1000)),
    }))
    .filter((entry) => entry.age_seconds >= limit)
    .sort((left, right) => right.age_seconds - left.age_seconds);
}

export function reportFor(rows, options = {}) {
  return {
    digest: deliveryDigest(rows, options),
    timeline: timeline(rows, options),
    stale: staleness(rows, options),
    generated_at: new Date(options.now ?? Date.now()).toISOString(),
  };
}

export function hostTable(rows) {
  const grouped = groupBy(rows, (row) => row.target ?? row.host);
  const table = [];
  for (const [host, entries] of grouped.entries()) {
    const counts = statusCounts(entries);
    table.push({
      host,
      rows: entries.length,
      delivered: counts.delivered ?? 0,
      failed: counts.failed ?? 0,
      success_rate: successRate(counts),
    });
  }
  table.sort((left, right) => right.rows - left.rows || (left.host < right.host ? -1 : 1));
  return table;
}

export function failureWindows(rows, options = {}) {
  const failures = rows.filter((row) => row.status === 'failed');
  const buckets = groupBy(failures, (row) => minuteKey(row.updated_at ?? row.created_at));
  return [...buckets.entries()]
    .map(([minute, entries]) => ({ minute, failures: entries.length }))
    .filter((entry) => entry.failures >= (options.minimum ?? 1))
    .sort((left, right) => (left.minute < right.minute ? -1 : 1));
}

export function compareReports(before, after) {
  return {
    digest: compareDigests(before.digest ?? before, after.digest ?? after),
    new_errors: (after.digest ?? after).errors
      .filter((entry) => !(before.digest ?? before).errors.some((seen) => seen.error === entry.error))
      .map((entry) => entry.error),
    stale_after: (after.stale ?? []).length,
  };
}

export function renderReport(report) {
  const lines = [renderDigest(report.digest)];
  if (report.stale.length > 0) {
    lines.push(`stale deliveries: ${report.stale.length}`);
    for (const entry of report.stale) lines.push(`stale ${entry.id} ${entry.age_seconds}s`);
  }
  return lines.join('\n');
}

export function trend(before, after) {
  const delta = (after.success_rate ?? 0) - (before.success_rate ?? 0);
  return {
    delta: Math.round(delta * 10) / 10,
    direction: delta === 0 ? 'flat' : delta > 0 ? 'up' : 'down',
  };
}

export function renderHostTable(rows) {
  const table = hostTable(rows);
  return [
    'host,rows,delivered,failed,success_rate',
    ...table.map((row) => `${row.host},${row.rows},${row.delivered},${row.failed},${row.success_rate}`),
  ].join('\n');
}

export function arrivalRate(rows, options = {}) {
  if (rows.length === 0) return 0;
  const from = new Date(options.from ?? rows[0].created_at).getTime();
  const to = new Date(options.to ?? rows[rows.length - 1].created_at).getTime();
  const minutes = Math.max(1, (to - from) / 60000);
  return Math.round((rows.length / minutes) * 100) / 100;
}
