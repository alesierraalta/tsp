import { escapeText, attrPair } from './escaping.js';
import { renderStatusPill, renderEnvironmentTag, renderReleaseTag, renderServiceTag } from './partials.js';
import { formatNumber } from '../util/numbers.js';
import { formatDuration, diffMinutes } from '../util/time.js';

export function table(headers, rows, options = {}) {
  const head = headers.map((header) => `<th ${attrPair('data-column', header.key ?? header)}>${escapeText(header.label ?? header)}</th>`).join('');
  const body = rows
    .map((row) => `<tr>${headers.map((header) => `<td>${header.render ? header.render(row) : escapeText(row[header.key] ?? '')}</td>`).join('')}</tr>`)
    .join('');
  return `<table class="grid ${options.className ?? ''}"><thead><tr>${head}</tr></thead><tbody>${body}</tbody></table>`;
}

export function releaseColumns() {
  return [
    { key: 'version', label: 'Version', render: (row) => renderReleaseTag(row) },
    { key: 'service', label: 'Service', render: (row) => renderServiceTag({ name: row.service }) },
    { key: 'environment', label: 'Environment', render: (row) => renderEnvironmentTag({ name: row.environment }) },
    { key: 'status', label: 'Status', render: (row) => renderStatusPill(row.status) },
    { key: 'created_at', label: 'Created' },
  ];
}

export function releasesTable(releases) {
  return table(releaseColumns(), releases ?? []);
}

export function deploymentColumns() {
  return [
    { key: 'service', label: 'Service', render: (row) => renderServiceTag({ name: row.service }) },
    { key: 'version', label: 'Version' },
    { key: 'environment', label: 'Environment', render: (row) => renderEnvironmentTag({ name: row.environment }) },
    { key: 'stage', label: 'Stage' },
    { key: 'status', label: 'Status', render: (row) => renderStatusPill(row.status) },
    { key: 'duration', label: 'Elapsed', render: (row) => escapeText(formatDuration(diffMinutes(row.started_at, row.finished_at ?? new Date()))) },
  ];
}

export function deploymentsTable(deployments) {
  return table(deploymentColumns(), deployments ?? []);
}

export function incidentColumns() {
  return [
    { key: 'title', label: 'Incident' },
    { key: 'severity', label: 'Severity' },
    { key: 'service', label: 'Service', render: (row) => renderServiceTag({ name: row.service }) },
    { key: 'state', label: 'State', render: (row) => renderStatusPill(row.state) },
    { key: 'owner', label: 'Owner' },
  ];
}

export function incidentsTable(incidents) {
  return table(incidentColumns(), incidents ?? []);
}

export function deliveryColumns() {
  return [
    { key: 'event', label: 'Event' },
    { key: 'host', label: 'Host' },
    { key: 'state', label: 'State', render: (row) => renderStatusPill(row.state) },
    { key: 'attempts', label: 'Attempts', render: (row) => escapeText(formatNumber(row.attempts)) },
  ];
}

export function deliveriesTable(deliveries) {
  return table(deliveryColumns(), deliveries ?? []);
}

export function countTable(rows, keyLabel, valueLabel) {
  return table(
    [
      { key: 'name', label: keyLabel },
      { key: 'value', label: valueLabel, render: (row) => escapeText(formatNumber(row.value)) },
    ],
    (rows ?? []).map((row) => ({ name: row.name ?? row.label ?? '', value: row.value ?? row.count ?? 0 })),
  );
}

export function emptyTable(message) {
  return `<p class="empty">${escapeText(message)}</p>`;
}

export function sortableHeader(label, column, options = {}) {
  const direction = options.direction === 'desc' ? 'desc' : 'asc';
  const next = direction === 'asc' ? 'desc' : 'asc';
  return `<a class="sort" href="?sort=${encodeURIComponent(column)}&direction=${next}">${escapeText(label)}<span class="direction">${escapeText(direction)}</span></a>`;
}

export function footerRow(cells) {
  return `<tfoot><tr>${(cells ?? []).map((cell) => `<td>${escapeText(cell)}</td>`).join('')}</tr></tfoot>`;
}

export function caption(text) {
  return `<caption>${escapeText(text)}</caption>`;
}

export function countSummary(rows, columns) {
  return `<p class="table-summary">${escapeText(String((rows ?? []).length))} rows, ${escapeText(String((columns ?? []).length))} columns</p>`;
}

export function pagedTable(rows, columns, page) {
  const start = Math.max(0, Math.trunc(page?.offset ?? 0));
  const size = Math.max(1, Math.trunc(page?.limit ?? 50));
  return table(columns, (rows ?? []).slice(start, start + size));
}

export function emptyRow(span, text) {
  return `<tr class="empty"><td colspan="${Math.max(1, Math.trunc(span))}">${escapeText(text)}</td></tr>`;
}
