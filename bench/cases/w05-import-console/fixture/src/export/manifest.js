import { fingerprint } from '../util/hash.js';
import { formatCents, sumCents } from '../money/decimal.js';

export function manifestFor(dataset, options = {}) {
  const rows = dataset.all();
  const columns = dataset.columns();
  const moneyColumns = dataset.schema.columns
    .filter((column) => column.type === 'money')
    .map((column) => column.name);
  return {
    dataset: dataset.schema.name,
    key: [...dataset.schema.key],
    columns,
    rows: rows.length,
    money_columns: moneyColumns,
    totals: Object.fromEntries(
      moneyColumns.map((column) => [column, sumCents(rows.map((row) => row[column] ?? 0))]),
    ),
    checksum: fingerprint([dataset.schema.name, rows.length, columns.join(',')]),
    generated_at: new Date(options.now ?? Date.now()).toISOString(),
    format: options.format ?? 'csv',
    version: 1,
  };
}

export function describeManifest(manifest) {
  const totals = Object.entries(manifest.totals ?? {})
    .map(([column, cents]) => `${column}=${formatCents(cents, 'USD')}`)
    .join(' ');
  return `${manifest.dataset} rows=${manifest.rows} columns=${manifest.columns.length} ${totals}`.trim();
}

export function compareManifests(left, right) {
  const changes = [];
  if (left.dataset !== right.dataset) changes.push('dataset');
  if (left.rows !== right.rows) changes.push('rows');
  if (left.columns.join(',') !== right.columns.join(',')) changes.push('columns');
  if (left.checksum !== right.checksum) changes.push('checksum');
  return { changed: changes.length > 0, fields: changes };
}

export function manifestLines(manifest) {
  return [
    `dataset: ${manifest.dataset}`,
    `rows: ${manifest.rows}`,
    `columns: ${manifest.columns.join(', ')}`,
    `checksum: ${manifest.checksum}`,
    `generated: ${manifest.generated_at}`,
  ];
}
