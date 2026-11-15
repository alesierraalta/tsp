import { toCsvCell } from './csv.js';
import { stableStringify } from '../util/json.js';

export function toCsv(rows, fields) {
  const columns = fields ?? Object.keys(rows[0] ?? {});
  const lines = [columns.map(toCsvCell).join(',')];
  for (const row of rows) lines.push(columns.map((column) => toCsvCell(row[column])).join(','));
  return `${lines.join('\n')}\n`;
}

export function toNdjson(rows) {
  return `${rows.map((row) => JSON.stringify(row)).join('\n')}\n`;
}

export function toJson(rows, meta = {}) {
  return stableStringify({ exported_at: meta.exported_at ?? null, rows });
}

export const EXPORT_FORMATS = ['csv', 'json', 'ndjson'];

export function exportRows(rows, format) {
  if (!EXPORT_FORMATS.includes(format)) throw new TypeError(`unknown export format ${format}`);
  if (format === 'csv') return toCsv(rows);
  if (format === 'ndjson') return toNdjson(rows);
  return toJson(rows);
}

export function rollupRows(points) {
  return points.map((point) => ({ bucket: point.bucket, events: point.events ?? 0, samples: point.samples ?? 0 }));
}

export function reportRows(tenant, points, totals) {
  return points.map((point, index) => ({
    tenant,
    bucket: point.bucket,
    events: point.events ?? 0,
    cumulative: points.slice(0, index + 1).reduce((total, row) => total + (row.events ?? 0), 0),
    share: totals === 0 ? 0 : Math.round(((point.events ?? 0) / totals) * 10000) / 100,
  }));
}

export function exportPlan(rows, limit = 5000) {
  const groups = [];
  for (let index = 0; index < rows.length; index += limit) groups.push(rows.slice(index, index + limit));
  return {
    rows: rows.length,
    files: Math.max(1, groups.length),
    bytes: Buffer.byteLength(JSON.stringify(rows), 'utf8'),
  };
}

export function fileNameFor(report, format, instant) {
  const stamp = new Date(instant ?? Date.now()).toISOString().slice(0, 10).replace(/-/g, '');
  return `${report}-${stamp}.${format}`;
}

// An export is described before it is written: the manifest names the report, the format, the columns and
// the byte count, so a partner can check that what arrived is what was promised.
export function manifestFor(report, rows, format, options = {}) {
  const text = exportRows(rows, format);
  return {
    report,
    format,
    columns: Object.keys(rows[0] ?? {}),
    rows: rows.length,
    bytes: Buffer.byteLength(text, 'utf8'),
    generated_at: new Date(options.now ?? Date.now()).toISOString(),
  };
}

export function chunkRows(rows, size = 500) {
  const chunks = [];
  const width = Math.max(1, Math.trunc(size));
  for (let index = 0; index < rows.length; index += width) chunks.push(rows.slice(index, index + width));
  return chunks;
}

export function chunkedExport(report, rows, format, size = 500) {
  return chunkRows(rows, size).map((slice, index) => ({
    name: `${report}-${String(index + 1).padStart(3, '0')}.${format}`,
    rows: slice.length,
    text: exportRows(slice, format),
  }));
}

export function exportSummary(files) {
  return {
    files: files.length,
    rows: files.reduce((total, file) => total + file.rows, 0),
    bytes: files.reduce((total, file) => total + Buffer.byteLength(file.text, 'utf8'), 0),
  };
}

export function isEmptyExport(rows) {
  return !Array.isArray(rows) || rows.length === 0;
}
