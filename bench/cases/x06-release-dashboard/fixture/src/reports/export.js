import { toCsvCell } from './csv.js';
import { stableStringify } from '../util/json.js';

export const EXPORT_FORMATS = ['csv', 'ndjson', 'json'];
export const EXPORT_REPORTS = ['releases', 'deployments', 'incidents', 'timeline', 'metrics'];

export function toCsv(rows, fields) {
  const columns = fields ?? Object.keys(rows?.[0] ?? {});
  const lines = [columns.map(toCsvCell).join(',')];
  for (const row of rows ?? []) lines.push(columns.map((column) => toCsvCell(row[column])).join(','));
  return `${lines.join('\n')}\n`;
}

export function toNdjson(rows) {
  return `${(rows ?? []).map((row) => JSON.stringify(row)).join('\n')}\n`;
}

export function toJson(rows, meta = {}) {
  return stableStringify({ exported_at: meta.exported_at ?? null, report: meta.report ?? null, rows: rows ?? [] });
}

export function renderRows(rows, format) {
  if (!EXPORT_FORMATS.includes(format)) throw new TypeError(`unknown export format ${format}`);
  if (format === 'csv') return toCsv(rows);
  if (format === 'ndjson') return toNdjson(rows);
  return toJson(rows);
}

export function chunkRows(rows, size = 500) {
  const width = Math.max(1, Math.trunc(size));
  const out = [];
  for (let index = 0; index < (rows ?? []).length; index += width) out.push(rows.slice(index, index + width));
  return out;
}

export function chunkedExport(report, rows, format, size = 500) {
  return chunkRows(rows, size).map((slice, index) => ({
    name: `${report}-${String(index + 1).padStart(3, '0')}.${format}`,
    rows: slice.length,
    text: renderRows(slice, format),
  }));
}

export function manifestFor(report, rows, format, options = {}) {
  const text = renderRows(rows, format);
  return {
    report,
    format,
    columns: Object.keys(rows?.[0] ?? {}),
    rows: (rows ?? []).length,
    bytes: Buffer.byteLength(text, 'utf8'),
    generated_at: new Date(options.now ?? Date.now()).toISOString(),
  };
}

export function exportSummary(files) {
  return {
    files: files.length,
    rows: files.reduce((total, file) => total + file.rows, 0),
    bytes: files.reduce((total, file) => total + Buffer.byteLength(file.text, 'utf8'), 0),
  };
}

export function fileNameFor(report, format, instant) {
  const stamp = new Date(instant ?? Date.now()).toISOString().slice(0, 10).replace(/-/g, '');
  return `${report}-${stamp}.${format}`;
}

export function isEmptyExport(rows) {
  return !Array.isArray(rows) || rows.length === 0;
}

export function releaseRows(releases) {
  return (releases ?? []).map((release) => ({
    id: release.id,
    version: release.version,
    service: release.service,
    environment: release.environment,
    status: release.status,
    channel: release.channel,
    created_at: release.created_at,
  }));
}

export function deploymentRows(deployments) {
  return (deployments ?? []).map((deployment) => ({
    id: deployment.id,
    release: deployment.release,
    service: deployment.service,
    version: deployment.version,
    environment: deployment.environment,
    stage: deployment.stage,
    status: deployment.status,
    started_at: deployment.started_at,
    finished_at: deployment.finished_at,
  }));
}

export function incidentRows(incidents) {
  return (incidents ?? []).map((incident) => ({
    id: incident.id,
    title: incident.title,
    severity: incident.severity,
    service: incident.service,
    state: incident.state,
    opened_at: incident.opened_at,
    resolved_at: incident.resolved_at,
  }));
}
