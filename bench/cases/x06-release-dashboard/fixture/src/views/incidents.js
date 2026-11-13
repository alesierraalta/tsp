import { page, section, twoColumn, empty, notice } from './layout.js';
import { card, statRow, summaryLine } from './components.js';
import { incidentColumns, table, emptyTable } from './tables.js';
import { renderBreadcrumbs, renderKeyValue, renderSeverityBadge, renderStatusPill, renderServiceTag, renderDuration } from './partials.js';
import { escapeText } from './escaping.js';
import { formatNumber } from '../util/numbers.js';
import { diffMinutes, formatDuration } from '../util/time.js';

export const SEVERITY_HELP = {
  sev1: 'Customer-facing outage; page the on-call and freeze production.',
  sev2: 'Degraded service or a failed rollout; block the next promotion.',
  sev3: 'A single feature is wrong; fix in the next release.',
  sev4: 'Cosmetic or internal; no release blocking.',
};

export function severityGuide() {
  return `<dl class="severities">${Object.entries(SEVERITY_HELP)
    .map(([severity, help]) => `<dt>${renderSeverityBadge(severity)}</dt><dd>${escapeText(help)}</dd>`)
    .join('')}</dl>`;
}

export function incidentHeader(incident) {
  return [
    '<header class="incident">',
    `<h2>${renderSeverityBadge(incident.severity)} ${escapeText(incident.title)}</h2>`,
    `<p>${renderServiceTag({ name: incident.service })} ${renderStatusPill(incident.state)}</p>`,
    summaryLine(`${incident.environment} · opened ${incident.opened_at}`),
    '</header>',
  ].join('');
}

export function incidentDetail(incident, options = {}) {
  const left = [
    section(
      'details',
      renderKeyValue({
        service: incident.service,
        environment: incident.environment,
        severity: incident.severity,
        state: incident.state,
        owner: incident.owner ?? 'unassigned',
        opened_at: incident.opened_at,
        resolved_at: incident.resolved_at ?? 'open',
      }),
    ),
    options.resolution ? section('resolution', `<p>${escapeText(options.resolution)}</p>`) : '',
  ]
    .filter(Boolean)
    .join('');
  const right = [
    section('timeline', options.timeline ?? empty('No related activity.')),
    section('guide', severityGuide()),
  ].join('');
  return twoColumn(left, right);
}

export function incidentPage(incident, options = {}) {
  return page({
    title: incident.title,
    section: 'incidents',
    tenant: options.tenant ?? incident.tenant,
    body: [
      renderBreadcrumbs([{ label: 'Incidents', href: '/incidents' }, { label: incident.id }]),
      options.notice ? notice(options.notice, options.noticeKind) : '',
      incidentHeader(incident),
      incidentDetail(incident, options),
    ]
      .filter(Boolean)
      .join(''),
  });
}

export function incidentsPage(incidents, options = {}) {
  const open = (incidents ?? []).filter((incident) => incident.state === 'open');
  const blocking = open.filter((incident) => ['sev1', 'sev2'].includes(incident.severity));
  return page({
    title: options.title ?? 'Incidents',
    section: 'incidents',
    tenant: options.tenant,
    body: [
      statRow([
        { label: 'open', value: open.length, format: formatNumber },
        { label: 'blocking', value: blocking.length, format: formatNumber },
        { label: 'total', value: (incidents ?? []).length, format: formatNumber },
      ]),
      section('list', (incidents ?? []).length === 0 ? emptyTable('No incidents recorded.') : table(incidentColumns(), incidents)),
    ].join(''),
  });
}

export function blockingPanel(incidents) {
  const blocking = (incidents ?? []).filter((incident) => incident.state === 'open' && ['sev1', 'sev2'].includes(incident.severity));
  if (blocking.length === 0) return card('Release blockers', empty('Nothing blocks a promotion.'));
  return card(
    'Release blockers',
    `<ul class="blockers">${blocking
      .map((incident) => `<li>${renderSeverityBadge(incident.severity)} ${escapeText(incident.title)} ${renderDuration(diffMinutes(incident.opened_at, new Date()))}</li>`)
      .join('')}</ul>`,
  );
}

export function incidentTable(incidents) {
  return table(incidentColumns(), incidents ?? []);
}

export function severityCounts(incidents) {
  const counts = {};
  for (const incident of incidents ?? []) counts[incident.severity] = (counts[incident.severity] ?? 0) + 1;
  return counts;
}

export function responseTime(incident) {
  if (!incident?.resolved_at) return formatDuration(diffMinutes(incident.opened_at, new Date()) * 60000);
  return formatDuration(diffMinutes(incident.opened_at, incident.resolved_at) * 60000);
}
