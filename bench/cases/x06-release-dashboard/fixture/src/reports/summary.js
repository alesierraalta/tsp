import { percentOf, roundTo } from '../util/numbers.js';
import { diffMinutes } from '../util/time.js';

export function releaseSummary(releases, options = {}) {
  const byStatus = {};
  const byEnvironment = {};
  const byService = {};
  for (const release of releases ?? []) {
    byStatus[release.status] = (byStatus[release.status] ?? 0) + 1;
    byEnvironment[release.environment] = (byEnvironment[release.environment] ?? 0) + 1;
    byService[release.service] = (byService[release.service] ?? 0) + 1;
  }
  return {
    tenant: options.tenant ?? null,
    releases: (releases ?? []).length,
    live: byStatus.live ?? 0,
    planned: byStatus.planned ?? 0,
    byStatus,
    byEnvironment,
    byService,
    environments: Object.keys(byEnvironment).length,
    services: Object.keys(byService).length,
  };
}

export function deploymentSummary(deployments, options = {}) {
  const finished = (deployments ?? []).filter((deployment) => ['succeeded', 'failed', 'rolled_back'].includes(deployment.status));
  const succeeded = finished.filter((deployment) => deployment.status === 'succeeded').length;
  const failed = finished.filter((deployment) => deployment.status === 'failed').length;
  const durations = finished
    .filter((deployment) => deployment.finished_at)
    .map((deployment) => diffMinutes(deployment.started_at, deployment.finished_at));
  return {
    tenant: options.tenant ?? null,
    deployments: (deployments ?? []).length,
    finished: finished.length,
    succeeded,
    failed,
    rolled_back: finished.filter((deployment) => deployment.status === 'rolled_back').length,
    open: (deployments ?? []).length - finished.length,
    success_rate: finished.length === 0 ? 0 : percentOf(succeeded, finished.length),
    mean_minutes: durations.length === 0 ? 0 : roundTo(durations.reduce((total, value) => total + value, 0) / durations.length, 2),
  };
}

export function combinedSummary(releases, deployments) {
  return { releases: releaseSummary(releases), deployments: deploymentSummary(deployments) };
}

export function summaryLines(summary) {
  return [
    `${summary.releases?.releases ?? 0} releases across ${summary.releases?.environments ?? 0} environments`,
    `${summary.deployments?.deployments ?? 0} deployments, ${summary.deployments?.success_rate ?? 0}% succeeded`,
  ];
}

export function busiestService(releases) {
  const summary = releaseSummary(releases);
  const entries = Object.entries(summary.byService).sort((left, right) => right[1] - left[1]);
  return entries[0] === undefined ? undefined : { service: entries[0][0], releases: entries[0][1] };
}

export function needsAttention(deployments, incidents) {
  const open = (deployments ?? []).filter((deployment) => deployment.status === 'awaiting_approval');
  const failed = (deployments ?? []).filter((deployment) => deployment.status === 'failed');
  return { awaiting_approval: open.length, failed: failed.length, incidents: (incidents ?? []).length };
}

export function summaryByEnvironment(deployments) {
  const buckets = new Map();
  for (const deployment of deployments ?? []) {
    const bucket = buckets.get(deployment.environment) ?? [];
    bucket.push(deployment);
    buckets.set(deployment.environment, bucket);
  }
  return [...buckets.entries()]
    .map(([environment, rows]) => ({ environment, ...deploymentSummary(rows) }))
    .sort((left, right) => right.deployments - left.deployments);
}

export function summaryByService(releases, deployments) {
  const services = new Set([...(releases ?? []).map((release) => release.service), ...(deployments ?? []).map((deployment) => deployment.service)]);
  return [...services]
    .map((service) => ({
      service,
      releases: (releases ?? []).filter((release) => release.service === service).length,
      deployments: (deployments ?? []).filter((deployment) => deployment.service === service).length,
    }))
    .sort((left, right) => right.deployments - left.deployments)
    .slice(0, 10);
}

export function tenantSummary(tenant, releases, deployments) {
  return {
    tenant,
    releases: releaseSummary(releases, { tenant }),
    deployments: deploymentSummary(deployments, { tenant }),
    environments: summaryByEnvironment(deployments),
  };
}
