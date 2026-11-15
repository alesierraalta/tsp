import { diffMinutes, weekKey, dayBucket } from '../util/time.js';
import { percentOf, roundTo, mean, median, quantile } from '../util/numbers.js';
import { countBy, groupBy } from '../util/collections.js';

export const METRICS = ['lead_time', 'deploy_frequency', 'change_failure_rate', 'mttr'];

export function leadTimeMinutes(deployment) {
  if (!deployment?.finished_at) return undefined;
  return diffMinutes(deployment.started_at, deployment.finished_at);
}

export function leadTime(deployments) {
  const values = (deployments ?? []).map((deployment) => leadTimeMinutes(deployment)).filter((value) => value !== undefined);
  if (values.length === 0) return { count: 0, mean: 0, median: 0, p90: 0 };
  return { count: values.length, mean: roundTo(mean(values), 2), median: median(values), p90: quantile(values, 0.9) };
}

export function deployFrequency(deployments, weeks = 4) {
  const buckets = groupBy((deployments ?? []).filter((deployment) => deployment.status === 'succeeded'), (deployment) => weekKey(deployment.started_at));
  const rows = [...buckets.entries()].map(([week, entries]) => ({ week, deployments: entries.length })).sort((left, right) => (left.week < right.week ? -1 : 1));
  const window = rows.slice(-Math.max(1, weeks));
  const total = window.reduce((sum, row) => sum + row.deployments, 0);
  return { weeks: window.length, total, per_week: window.length === 0 ? 0 : roundTo(total / window.length, 2), rows };
}

export function changeFailureRate(deployments) {
  const finished = (deployments ?? []).filter((deployment) => ['succeeded', 'failed', 'rolled_back'].includes(deployment.status));
  const failed = finished.filter((deployment) => deployment.status !== 'succeeded').length;
  return { finished: finished.length, failed, rate: percentOf(failed, finished.length) };
}

export function mttr(incidents) {
  const closed = (incidents ?? []).filter((incident) => incident.resolved_at);
  const values = closed.map((incident) => diffMinutes(incident.opened_at, incident.resolved_at));
  return {
    incidents: closed.length,
    mean_minutes: values.length === 0 ? 0 : roundTo(values.reduce((total, value) => total + value, 0) / values.length, 2),
    open: (incidents ?? []).filter((incident) => incident.state === 'open').length,
  };
}

export function metricTable(deployments, incidents) {
  const lead = leadTime(deployments);
  const frequency = deployFrequency(deployments);
  const failure = changeFailureRate(deployments);
  const recovery = mttr(incidents);
  return [
    { name: 'lead_time', value: lead.median, unit: 'minutes', note: `${lead.count} finished deployments` },
    { name: 'deploy_frequency', value: frequency.per_week, unit: 'per week', note: `${frequency.total} succeeded` },
    { name: 'change_failure_rate', value: failure.rate, unit: 'percent', note: `${failure.failed} of ${failure.finished}` },
    { name: 'mttr', value: recovery.mean_minutes, unit: 'minutes', note: `${recovery.incidents} resolved incidents` },
  ];
}

export function deploymentsByDay(deployments) {
  const buckets = countBy((deployments ?? []).map((deployment) => dayBucket(deployment.started_at)), (day) => day);
  return [...buckets.entries()].map(([day, count]) => ({ day, deployments: count })).sort((left, right) => (left.day < right.day ? -1 : 1));
}

export function busiestDay(deployments) {
  return deploymentsByDay(deployments).reduce((best, row) => (best === undefined || row.deployments > best.deployments ? row : best), undefined);
}

export function metricsReport(deployments, incidents) {
  return {
    lead_time: leadTime(deployments),
    frequency: deployFrequency(deployments),
    failure: changeFailureRate(deployments),
    recovery: mttr(incidents),
    table: metricTable(deployments, incidents),
    busiest: busiestDay(deployments),
  };
}

export function windowedMetrics(deployments, from, to) {
  const inWindow = (deployments ?? []).filter((deployment) => {
    const at = new Date(deployment.started_at).getTime();
    return at >= new Date(from).getTime() && at <= new Date(to).getTime();
  });
  return {
    deployments: inWindow.length,
    lead_time: leadTime(inWindow),
    failure: changeFailureRate(inWindow),
  };
}

export function doraTable(deployments, incidents) {
  const lead = leadTime(deployments);
  const frequency = deployFrequency(deployments);
  const failure = changeFailureRate(deployments);
  const recovery = mttr(incidents);
  return [
    { measure: 'lead time for changes', value: lead.median, unit: 'minutes', band: bandOf(lead.median, [60, 1440, 10080]) },
    { measure: 'deployment frequency', value: frequency.per_week, unit: 'per week', band: bandOf(-frequency.per_week, [-7, -1, -0.25]) },
    { measure: 'change failure rate', value: failure.rate, unit: 'percent', band: bandOf(failure.rate, [10, 20, 30]) },
    { measure: 'time to restore', value: recovery.mean_minutes, unit: 'minutes', band: bandOf(recovery.mean_minutes, [60, 240, 1440]) },
  ];
}

export function bandOf(value, thresholds) {
  const number = Number(value) || 0;
  const [elite, high, medium] = thresholds;
  if (number <= elite) return 'elite';
  if (number <= high) return 'high';
  if (number <= medium) return 'medium';
  return 'low';
}

export function failureRateByEnvironment(deployments) {
  const buckets = groupBy(deployments ?? [], (deployment) => deployment.environment);
  return [...buckets.entries()].map(([environment, rows]) => ({ environment, ...changeFailureRate(rows) })).sort((left, right) => right.rate - left.rate);
}

export function leadTimeTrend(deployments) {
  const byWeek = groupBy((deployments ?? []).filter((deployment) => deployment.finished_at), (deployment) => weekKey(deployment.started_at));
  return [...byWeek.entries()]
    .map(([week, rows]) => ({ week, median: leadTime(rows).median, deployments: rows.length }))
    .sort((left, right) => (left.week < right.week ? -1 : 1));
}
