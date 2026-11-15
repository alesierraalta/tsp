import { page, section, twoColumn, empty, notice } from './layout.js';
import { card, link, statRow, summaryLine, legend } from './components.js';
import { releasesTable, deploymentsTable, emptyTable, table } from './tables.js';
import { renderChangelog, renderEnvironmentTag, renderKeyValue, renderReleaseTag, renderServiceTag, renderStatusPill, renderBreadcrumbs, renderDuration } from './partials.js';
import { escapeText } from './escaping.js';
import { formatNumber } from '../util/numbers.js';
import { diffMinutes, formatDuration } from '../util/time.js';

export function releaseHeader(release) {
  return [
    '<header class="release">',
    `<h2>${renderServiceTag({ name: release.service })} ${renderReleaseTag(release)}</h2>`,
    `<p>${renderEnvironmentTag({ name: release.environment })} ${renderStatusPill(release.status)}</p>`,
    summaryLine(release.summary ?? ''),
    '</header>',
  ].join('');
}

export function releaseList(releases) {
  if ((releases ?? []).length === 0) return emptyTable('No releases yet.');
  return releasesTable(releases);
}

export function releaseDetail(release, deployments = []) {
  const left = [
    section('meta', renderKeyValue({
      service: release.service,
      version: release.version,
      channel: release.channel,
      environment: release.environment,
      status: release.status,
      created_by: release.created_by,
      created_at: release.created_at,
    })),
    section('changelog', renderChangelog(release.changelog ?? [])),
  ].join('');
  const right = [
    section('deployments', deployments.length === 0 ? empty('This release has not been deployed.') : deploymentsTable(deployments)),
    section('actions', `<p>${escapeText('Publish, promote or roll back through the API.')}</p>`),
  ].join('');
  return twoColumn(left, right);
}

export function releasePage(release, options = {}) {
  return page({
    title: `${release.service} ${release.version}`,
    subtitle: options.subtitle,
    section: 'releases',
    tenant: release.tenant,
    body: [
      renderBreadcrumbs([{ label: 'Releases', href: '/releases' }, { label: `${release.service} ${release.version}` }]),
      options.notice ? notice(options.notice, options.noticeKind) : '',
      releaseHeader(release),
      releaseDetail(release, options.deployments ?? []),
    ]
      .filter(Boolean)
      .join(''),
  });
}

export function releasesPage(releases, options = {}) {
  const byEnvironment = {};
  for (const release of releases ?? []) byEnvironment[release.environment] = (byEnvironment[release.environment] ?? 0) + 1;
  return page({
    title: options.title ?? 'Releases',
    section: 'releases',
    tenant: options.tenant,
    body: [
      statRow([
        { label: 'releases', value: (releases ?? []).length, format: formatNumber },
        { label: 'environments', value: Object.keys(byEnvironment).length, format: formatNumber },
      ]),
      section('list', releaseList(releases)),
    ].join(''),
  });
}

export function promotionTrail(release, history = []) {
  const rows = history.map((entry) => ({
    from: entry.from,
    to: entry.to,
    at: entry.at,
    minutes: diffMinutes(entry.at, entry.finished_at ?? entry.at),
  }));
  return table(
    [
      { key: 'from', label: 'From', render: (row) => renderEnvironmentTag({ name: row.from }) },
      { key: 'to', label: 'To', render: (row) => renderEnvironmentTag({ name: row.to }) },
      { key: 'minutes', label: 'Took', render: (row) => renderDuration(row.minutes) },
    ],
    rows,
  );
}

export function legacyNotes(release) {
  return `${release.service} ${release.version} (${release.environment}) — ${release.summary ?? ''}`;
}

export function releaseCard(release) {
  return card(
    `${release.service} ${release.version}`,
    renderChangelog(release.changelog ?? []),
    { footer: link('details', `/releases/${release.id}`) },
  );
}

export function releaseMetricsPanel(deployment) {
  if (!deployment) return section('metrics', empty('No deployment for this release.'));
  return section('metrics', renderKeyValue({
    stage: deployment.stage,
    elapsed: formatDuration(diffMinutes(deployment.started_at, deployment.finished_at ?? new Date()) * 60000),
    approvals: deployment.approvals?.length ?? 0,
  }));
}

export function statusLegend() {
  return legend([
    { kind: 'planned', label: 'planned' },
    { kind: 'live', label: 'live' },
    { kind: 'rolled_back', label: 'rolled back' },
    { kind: 'cancelled', label: 'cancelled' },
  ]);
}
