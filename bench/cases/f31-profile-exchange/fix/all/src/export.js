import { selection } from './schema.js';
import { filterRows, sortRows, projectRows } from './query.js';

export function csvCell(value) {
  const text = value === null || value === undefined ? '' : String(value);
  if (/[",\r\n]/.test(text)) return `"${text.replaceAll('"', '""')}"`;
  return text;
}

export function csvLine(values) {
  return values.map(csvCell).join(',');
}

export function exportCSV(rows, columns, filter = {}) {
  const chosen = selection(columns);
  const selected = projectRows(sortRows(filterRows(rows, filter)), chosen);
  const lines = [csvLine(chosen)];
  for (const row of selected) {
    const values = chosen.map(column => row[column]);
    lines.push(csvLine(values));
  }
  return lines.join('\n');
}

export function exportJSON(rows, columns, filter = {}) {
  const chosen = selection(columns);
  return JSON.stringify(projectRows(sortRows(filterRows(rows, filter)), chosen));
}

export function describeExport(rows, columns, filter = {}) {
  const chosen = selection(columns);
  const count = filterRows(rows, filter).length;
  return { columns: chosen, count, mediaType: 'text/csv' };
}

export function chunks(text, size = 4096) {
  if (!Number.isSafeInteger(size) || size < 1) throw new TypeError('invalid chunk size');
  const result = [];
  for (let offset = 0; offset < text.length; offset += size) {
    result.push(text.slice(offset, offset + size));
  }
  return result;
}
