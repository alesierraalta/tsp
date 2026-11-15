import { groupBy, sortBy } from '../util/collections.js';
import { dayBucket, diffMinutes } from '../util/time.js';
import { percentOf, roundTo } from '../util/numbers.js';
import { severityRank } from '../domain/incidents.js';

// A health report answers "is this service safe to release to" from what the dashboard already stores:
// open incidents, failed deployments and the age of the last successful rollout.
export const HEALTH_STATES = ['healthy', 'watch', 'degraded'];

export function serviceHealth(service, deployments, incidents, options = {}) {
  const scoped = (deployments ?? []).filter((deployment) => deployment.service === service);
  const open = (incidents ?? []).filter((incident) => incident.service === service && incident.state === 'open');
  const blocking = open.filter((incident) => severityRank(incident.severity) >= severityRank('sev2'));
  const succeeded = scoped.filter((deployment) => deployment.status === 'succeeded');
  const lastSuccess = sortBy(succeeded, (deployment) => deployment.finished_at ?? deployment.started_at, 'desc')[0];
  const failed = scoped.filter((deployment) => deployment.status === 'failed');
  const state = blocking.length > 0 ? 'degraded' : failed.length > 0 || open.length > 0 ? 'watch' : 'healthy';
  return {
    service,
    state,
    incidents: open.length,
    blocking: blocking.length,
    deployments: scoped.length,
    failed: failed.length,
    success_rate: percentOf(succeeded.length, scoped.length || 1),
    last_success: lastSuccess?.finished_at ?? null,
    stale_days: lastSuccess === undefined ? undefined : roundTo(diffMinutes(lastSuccess.finished_at ?? lastSuccess.started_at, new Date(options.now ?? Date.now())) / 1440, 1),
  };
}

export function healthTable(services, deployments, incidents, options = {}) {
  return (services ?? [])
    .map((service) => serviceHealth(typeof service === 'string' ? service : service.id ?? service.name, deployments, incidents, options))
    .sort((left, right) => HEALTH_STATES.indexOf(right.state) - HEALTH_STATES.indexOf(left.state));
}

export function overallHealth(rows) {
  if ((rows ?? []).some((row) => row.state === 'degraded')) return 'degraded';
  if ((rows ?? []).some((row) => row.state === 'watch')) return 'watch';
  return 'healthy';
}

export function healthReport(services, deployments, incidents, options = {}) {
  const rows = healthTable(services, deployments, incidents, options);
  return {
    services: rows.length,
    overall: overallHealth(rows),
    degraded: rows.filter((row) => row.state === 'degraded').length,
    watch: rows.filter((row) => row.state === 'watch').length,
    rows,
  };
}

export function dailyFailures(deployments) {
  const buckets = groupBy((deployments ?? []).filter((deployment) => deployment.status === 'failed'), (deployment) => dayBucket(deployment.started_at));
  return [...buckets.entries()].map(([day, rows]) => ({ day, failures: rows.length })).sort((left, right) => (left.day < right.day ? -1 : 1));
}

export function worstService(rows) {
  return sortBy(rows ?? [], (row) => row.blocking ?? 0, 'desc')[0];
}

export function healthyShare(rows) {
  return percentOf((rows ?? []).filter((row) => row.state === 'healthy').length, (rows ?? []).length || 1);
}
