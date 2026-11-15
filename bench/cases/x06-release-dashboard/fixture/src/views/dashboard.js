import { page, section, twoColumn, notice, empty } from './layout.js';
import { card, statRow, sparkline, progress } from './components.js';
import { deliveriesTable, deploymentsTable, emptyTable, incidentColumns, table } from './tables.js';
import { renderEnvironmentTag, renderHostList, renderStatusPill, renderServiceTag, renderKeyValue } from './partials.js';
import { escapeText } from './escaping.js';
import { formatNumber, percentOf } from '../util/numbers.js';
import { formatDuration, diffMinutes, dayBucket } from '../util/time.js';

export function environmentLabels(environments, labels = {}) {
  return (environments ?? []).map((name) => ({ name, label: labels[name] ?? name }));
}

export function environmentHeader(environments, labels, current) {
  const tabs = environmentLabels(environments, labels).map((environment) => {
    const active = environment.name === current ? ' tab active' : ' tab';
    return `<a class="${active.trim()}" href="?environment=${escapeText(environment.name)}">${renderEnvironmentTag(environment)}</a>`;
  });
  return `<div class="tabs">${tabs.join('')}</div>`;
}

export function summaryPanel(summary) {
  return section(
    'summary',
    statRow([
      { label: 'releases', value: summary.releases ?? 0, format: formatNumber },
      { label: 'deployments', value: summary.deployments ?? 0, format: formatNumber },
      { label: 'open', value: summary.open ?? 0, format: formatNumber },
      { label: 'failed', value: summary.failed ?? 0, format: formatNumber },
    ]),
  );
}

export function activityPanel(points) {
  const events = (points ?? []).map((point) => point.events ?? 0);
  const body = events.length === 0 ? empty('No activity in this window.') : `${sparkline(events)}${deploymentsTable(points.flatMap((point) => point.deployments ?? []))}`;
  return section('activity', body);
}

export function deploymentsPanel(deployments) {
  const body = (deployments ?? []).length === 0 ? emptyTable('No deployments recorded.') : deploymentsTable(deployments);
  return section('deployments', body);
}

export function deliveriesPanel(deliveries) {
  const body = (deliveries ?? []).length === 0 ? empty('No webhook deliveries yet.') : deliveriesTable(deliveries);
  return section('webhooks', body);
}

export function incidentsPanel(incidents) {
  const body = (incidents ?? []).length === 0 ? empty('No open incidents.') : table(incidentColumns(), incidents);
  return section('incidents', body);
}

export function releaseProgressPanel(deployment) {
  if (!deployment) return section('progress', empty('Nothing is deploying right now.'));
  const total = deployment.plan?.length ?? 1;
  const body = [
    `<p>${escapeText(deployment.service)} ${escapeText(deployment.version)} to ${renderEnvironmentTag({ name: deployment.environment })}</p>`,
    progress(deployment.stage_index, total),
    renderKeyValue({
      stage: deployment.stage,
      status: deployment.status,
      elapsed: formatDuration(diffMinutes(deployment.started_at, new Date()) * 60000),
    }),
  ].join('');
  return section('progress', body);
}

export function dashboardBody(view) {
  const left = [summaryPanel(view.summary ?? {}), deploymentsPanel(view.deployments ?? []), incidentsPanel(view.incidents ?? [])].join('');
  const right = [releaseProgressPanel(view.current), deliveriesPanel(view.deliveries ?? []), section('hosts', renderHostList(view.hosts ?? {}))].join('');
  return twoColumn(left, right);
}

export function dashboardPage(view) {
  return page({
    title: view.title ?? 'Release dashboard',
    subtitle: `${formatNumber(view.summary?.releases ?? 0)} releases across ${formatNumber(Object.keys(view.byEnvironment ?? {}).length)} environments`,
    section: 'dashboard',
    tenant: view.tenant,
    body: [
      environmentHeader(view.environments ?? [], view.environmentLabels ?? {}, view.environment),
      view.notice ? notice(view.notice, view.noticeKind) : '',
      dashboardBody(view),
    ]
      .filter(Boolean)
      .join(''),
  });
}

export function activityRows(deployments) {
  const byDay = new Map();
  for (const deployment of deployments ?? []) {
    const day = dayBucket(deployment.started_at);
    const bucket = byDay.get(day) ?? { day, events: 0, deployments: [] };
    bucket.events += 1;
    bucket.deployments.push(deployment);
    byDay.set(day, bucket);
  }
  return [...byDay.values()].sort((left, right) => (left.day < right.day ? -1 : 1));
}

export function dashboardStats(deployments, releases, incidents) {
  const failed = (deployments ?? []).filter((deployment) => deployment.status === 'failed').length;
  return {
    releases: (releases ?? []).length,
    deployments: (deployments ?? []).length,
    open: (deployments ?? []).filter((deployment) => !['succeeded', 'failed', 'rolled_back'].includes(deployment.status)).length,
    failed,
    failure_rate: percentOf(failed, (deployments ?? []).length),
    incidents: (incidents ?? []).length,
  };
}

export function servicePanel(services) {
  const rows = (services ?? []).map((service) => ({
    name: service.name,
    owner: service.owner,
    language: service.language,
  }));
  return section(
    'services',
    card(
      'Services',
      table(
        [
          { key: 'name', label: 'Service', render: (row) => renderServiceTag({ name: row.name }) },
          { key: 'owner', label: 'Owner' },
          { key: 'language', label: 'Language' },
        ],
        rows,
      ),
    ),
  );
}

export function statusStrip(counts) {
  const entries = Object.entries(counts ?? {});
  if (entries.length === 0) return empty('Nothing recorded.');
  return `<div class="strip">${entries.map(([status, count]) => `<span class="strip-item">${renderStatusPill(status)} ${escapeText(formatNumber(count))}</span>`).join('')}</div>`;
}
