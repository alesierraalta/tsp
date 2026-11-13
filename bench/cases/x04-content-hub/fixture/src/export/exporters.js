import { mkdir, readFile, readdir, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { invalidField, notFound } from '../util/errors.js';
import { assertPlainName, resolveUnder } from '../media/paths.js';
import { stableStringify } from '../util/json.js';
import { toCsv } from './csv.js';

export const FORMATS = ['json', 'csv', 'ndjson'];

export function flattenEntry(entry, fields) {
  const row = {
    id: entry.id,
    slug: entry.slug,
    locale: entry.locale,
    status: entry.status,
    version: entry.version,
    created_at: entry.created_at,
    updated_at: entry.updated_at,
  };
  for (const field of fields) {
    const value = entry.values?.[field];
    row[field] = value === undefined || value === null ? '' : value;
  }
  return row;
}

export class Exporter {
  constructor(options = {}) {
    this.entries = options.entries;
    this.schemas = options.schemas;
    this.root = options.root;
    this.bus = options.bus;
    this.clock = options.clock ?? (() => new Date());
  }

  rowsOf(apiId, options = {}) {
    const schema = this.schemas.get(apiId);
    if (!schema) throw notFound(`content type ${apiId}`);
    const fields = schema.fields.filter((field) => field.system !== true).map((field) => field.name);
    const records = this.entries.list(schema, { locale: options.locale, status: options.status });
    return records.map((entry) => flattenEntry(entry, fields));
  }

  render(rows, format) {
    if (!FORMATS.includes(format)) throw invalidField('format', `must be one of ${FORMATS.join(', ')}`);
    if (format === 'json') return stableStringify({ exported_at: new Date(this.clock()).toISOString(), rows });
    if (format === 'ndjson') return `${rows.map((row) => JSON.stringify(row)).join('\n')}\n`;
    return toCsv(rows);
  }

  async write(name, body) {
    const clean = assertPlainName(name);
    await mkdir(this.root, { recursive: true });
    const file = resolveUnder(this.root, clean);
    await writeFile(file, body);
    const written = { name: clean, bytes: Buffer.byteLength(body), written_at: new Date(this.clock()).toISOString() };
    if (this.bus) await this.bus.emit('export.afterWrite', { file: written });
    return written;
  }

  async exportType(apiId, format = 'json', options = {}) {
    const rows = this.rowsOf(apiId, options);
    const body = this.render(rows, format);
    const name = options.name ?? `${apiId}.${format === 'ndjson' ? 'ndjson' : format}`;
    const written = await this.write(name, body);
    return { ...written, rows: rows.length, format };
  }

  async list() {
    try {
      const entries = await readdir(this.root, { withFileTypes: true });
      return entries.filter((entry) => entry.isFile()).map((entry) => entry.name).sort();
    } catch (error) {
      if (error.code === 'ENOENT') return [];
      throw error;
    }
  }

  async read(name) {
    const clean = assertPlainName(name);
    const file = resolveUnder(this.root, clean);
    return readFile(file, 'utf8').catch(() => {
      throw notFound(`export ${name}`);
    });
  }

  async pathOf(name) {
    return join(this.root, assertPlainName(name));
  }
}

export function exportSummary(result) {
  return { name: result.name, rows: result.rows, format: result.format, bytes: result.bytes };
}

export function collectFields(rows) {
  const fields = new Set();
  for (const row of rows) for (const key of Object.keys(row)) fields.add(key);
  return [...fields];
}

export function alignRows(rows, fields) {
  return rows.map((row) => Object.fromEntries(fields.map((field) => [field, row[field] ?? ''])));
}

export function exportPlan(rows, format, options = {}) {
  const fields = collectFields(rows);
  const estimated = Buffer.byteLength(rows.map((row) => JSON.stringify(row)).join('\n'));
  return {
    format,
    rows: rows.length,
    fields: fields.length,
    bytes: estimated,
    needsSplitting: estimated > (options.maxBytes ?? 4 * 1024 * 1024),
  };
}

export function splitRows(rows, size = 500) {
  const out = [];
  for (let index = 0; index < rows.length; index += size) out.push(rows.slice(index, index + size));
  return out;
}

export function exportFileName(apiId, format, instant) {
  const stamp = new Date(instant ?? Date.now()).toISOString().slice(0, 10).replace(/-/g, '');
  return `${apiId}-${stamp}.${format === 'ndjson' ? 'ndjson' : format}`;
}

export function readBack(rows, fields) {
  const missing = [];
  for (const row of rows) {
    for (const field of fields) {
      if (row[field] === undefined) missing.push({ slug: row.slug, field });
    }
  }
  return missing;
}
