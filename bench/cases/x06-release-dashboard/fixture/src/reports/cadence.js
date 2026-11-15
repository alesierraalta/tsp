import { groupBy, countBy, sortBy } from '../util/collections.js';
import { dayBucket, weekKey, monthKey } from '../util/time.js';

const weekBucketStart = (value) => weekKey(value);
import { percentOf, roundTo } from '../util/numbers.js';

export function cadenceByWeek(releases) {
  const buckets = groupBy(releases ?? [], (release) => weekKey(release.created_at));
  return [...buckets.entries()]
    .map(([week, rows]) => ({ week, releases: rows.length, services: new Set(rows.map((row) => row.service)).size }))
    .sort((left, right) => (left.week < right.week ? -1 : 1));
}

export function cadenceByMonth(releases) {
  const buckets = countBy(releases ?? [], (release) => monthKey(release.created_at));
  return [...buckets.entries()].map(([month, releases]) => ({ month, releases })).sort((left, right) => (left.month < right.month ? -1 : 1));
}

export function cadenceByService(releases) {
  const buckets = groupBy(releases ?? [], (release) => release.service);
  const rows = [...buckets.entries()].map(([service, entries]) => ({
    service,
    releases: entries.length,
    days: new Set(entries.map((entry) => dayBucket(entry.created_at))).size,
    last: sortBy(entries, (entry) => entry.created_at, 'desc')[0]?.created_at,
  }));
  return rows.sort((left, right) => right.releases - left.releases);
}

export function releaseInterval(releases, service) {
  const entries = sortBy((releases ?? []).filter((release) => service === undefined || release.service === service), (release) => release.created_at, 'asc');
  if (entries.length < 2) return { releases: entries.length, mean_hours: 0, shortest_hours: 0 };
  const gaps = [];
  for (let index = 1; index < entries.length; index += 1) {
    gaps.push((new Date(entries[index].created_at).getTime() - new Date(entries[index - 1].created_at).getTime()) / 3600000);
  }
  return {
    releases: entries.length,
    mean_hours: roundTo(gaps.reduce((total, gap) => total + gap, 0) / gaps.length, 2),
    shortest_hours: roundTo(Math.min(...gaps), 2),
  };
}

export function churn(releases) {
  const byService = cadenceByService(releases);
  const total = (releases ?? []).length;
  return byService.map((row) => ({ service: row.service, share: percentOf(row.releases, total) }));
}

export function cadenceReport(releases) {
  const weeks = cadenceByWeek(releases);
  return {
    releases: (releases ?? []).length,
    weeks: weeks.length,
    per_week: weeks.length === 0 ? 0 : roundTo(weeks.reduce((total, week) => total + week.releases, 0) / weeks.length, 2),
    services: cadenceByService(releases).length,
    churn: churn(releases),
    latest_week: weeks[weeks.length - 1] ?? null,
  };
}

export function busiestWeek(releases) {
  return cadenceByWeek(releases).reduce((best, row) => (best === undefined || row.releases > best.releases ? row : best), undefined);
}

export function weekStarts(releases) {
  return [...new Set((releases ?? []).map((release) => weekBucketStart(release.created_at)))].sort();
}
