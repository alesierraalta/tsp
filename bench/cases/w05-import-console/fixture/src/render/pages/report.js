import { renderKeyValue, renderTable } from '../components/table.js';
import { renderStatGrid } from '../components/card.js';
import { renderNotice } from '../layout.js';
import { formatCents } from '../../money/decimal.js';

export function renderReportPage(model) {
  const rows = model.rows ?? [];
  const columns = model.columns ?? [];
  return [
    '<section class="report">',
    renderNotice(`Showing ${rows.length} of ${model.total ?? rows.length} rows`, { kind: 'info' }),
    renderStatGrid(
      [
        { label: 'Rows', value: model.total ?? rows.length, trend: 'flat' },
        { label: 'Total', value: model.totalCents ?? 0, money: true },
        { label: 'Pages', value: model.pages ?? 1, trend: 'flat' },
      ],
      { currency: model.currency },
    ),
    renderTable({
      columns,
      rows,
      currency: model.currency,
      caption: model.caption ?? 'Report rows',
    }),
    renderKeyValue(
      [
        { label: 'Dataset', value: model.dataset },
        { label: 'Generated', value: model.generatedAt },
        { label: 'Checksum', value: model.checksum },
      ],
      { currency: model.currency },
    ),
    '</section>',
  ].join('\n');
}

export function renderReportSummary(summary, options = {}) {
  const currency = options.currency ?? 'USD';
  const lines = [
    `<h2>${options.heading ?? 'Summary'}</h2>`,
    '<dl class="summary">',
    `<dt>Rows</dt><dd>${summary.rows}</dd>`,
    `<dt>Total</dt><dd>${formatCents(summary.totalCents ?? 0, currency)}</dd>`,
    `<dt>Average</dt><dd>${formatCents(summary.averageCents ?? 0, currency)}</dd>`,
    '</dl>',
  ];
  return lines.join('');
}

export function renderGroupTable(groups, options = {}) {
  const columns = [
    { key: 'label', label: options.label ?? 'Group' },
    { key: 'count', label: 'Rows', integer: true },
    { key: 'total', label: 'Total', money: true },
  ];
  return renderTable({ columns, rows: groups, currency: options.currency, caption: options.caption });
}

export function renderReportLinks(reports) {
  const items = reports.map(
    (report) =>
      `<li><a href="${report.href}" data-kind="${report.kind ?? 'report'}">${report.label}</a> <span class="count">${report.rows}</span></li>`,
  );
  return `<ul class="report-links">${items.join('')}</ul>`;
}

export function renderEmptyState(message, action) {
  return [
    '<div class="empty-state">',
    `<p>${message}</p>`,
    action ? `<a class="button" href="${action.href}">${action.label}</a>` : '',
    '</div>',
  ].join('');
}
