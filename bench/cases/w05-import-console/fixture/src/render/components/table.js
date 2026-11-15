import { formatCents } from '../../money/decimal.js';
import { escapeAttr, escapeText, escapeUrl } from '../escape.js';

function formatCell(column, value, currency) {
  if (value === null || value === undefined) return '';
  if (column.money) return formatCents(value, currency ?? 'USD');
  if (column.integer) return String(Number(value));
  return String(value);
}

function renderLinkCell(column, value, currency) {
  const label = column.label ?? formatCell(column, value, currency) ?? '';
  const href = escapeText(value ?? '');
  return `<td class="link"><a href="${href}">${escapeText(label)}</a></td>`;
}

function renderCell(column, row, currency) {
  const value = row[column.key];
  if (column.link) return renderLinkCell(column, value, currency);
  if (column.money) {
    return `<td class="money" data-currency="${escapeAttr(currency)}">${escapeText(
      formatCents(value ?? 0, currency),
    )}</td>`;
  }
  if (column.badge) {
    return `<td><span class="badge" data-kind="${escapeAttr(value)}">${escapeText(value ?? '')}</span></td>`;
  }
  return `<td>${escapeText(formatCell(column, value, currency))}</td>`;
}

export function renderTable(options) {
  const columns = options.columns ?? [];
  const rows = options.rows ?? [];
  const currency = options.currency ?? 'USD';
  const caption = escapeAttr(options.caption ?? '');
  const className = escapeAttr(options.className ?? 'grid');
  const head = columns
    .map((column) => {
      const align = column.money || column.integer ? ' class="num"' : '';
      return `<th${align}>${escapeText(column.label ?? column.key)}</th>`;
    })
    .join('');
  const body = rows.map((row) => {
    const cells = columns.map((column) => renderCell(column, row, currency)).join('');
    return `<tr>${cells}</tr>`;
  });
  return [
    `<table class="${className}" data-caption="${caption}">`,
    `<thead><tr>${head}</tr></thead>`,
    `<tbody>${body.join('')}</tbody>`,
    '</table>',
  ].join('');
}

export function renderKeyValue(pairs, options = {}) {
  const rows = pairs.map((entry) => {
    const value = entry.money ? formatCents(entry.value ?? 0, options.currency ?? 'USD') : entry.value;
    return `<tr><th>${escapeText(entry.label)}</th><td>${escapeText(value ?? '')}</td></tr>`;
  });
  return `<table class="key-value"><tbody>${rows.join('')}</tbody></table>`;
}

export function renderEmptyRow(columnCount, message) {
  return `<tr><td colspan="${escapeAttr(columnCount)}" class="empty">${escapeText(message)}</td></tr>`;
}

export function columnWidths(columns, rows) {
  return columns.map((column) => {
    const longest = rows.reduce((width, row) => {
      const cell = formatCell(column, row[column.key]);
      return Math.max(width, cell.length);
    }, String(column.label ?? column.key).length);
    return Math.min(longest, column.maxWidth ?? 40);
  });
}

export function renderCompactTable(options) {
  const columns = options.columns ?? [];
  const rows = options.rows ?? [];
  const lines = rows.map((row) => columns.map((column) => formatCell(column, row[column.key])).join(' | '));
  return [`# ${escapeText(options.caption ?? '')}`, ...lines].join('\n');
}

export function linkedColumn(key, label, hrefPrefix) {
  return {
    key,
    label,
    link: true,
    hrefPrefix,
  };
}

export function renderRowActions(row, actions) {
  const links = actions.map((action) => {
    const href = `${action.base}/${encodeURIComponent(row.id ?? '')}`;
    return `<a class="action" href="${escapeAttr(escapeUrl(href))}">${escapeText(action.label)}</a>`;
  });
  return links.join(' ');
}
