import { readFile, readdir, stat, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { notFound } from '../http/errors.js';
import { assertPlainName, resolveUnder } from '../files/paths.js';

const MAX_ROWS = 5000;

export class ReportArchive {
  constructor(options = {}) {
    this.root = options.root;
    if (!this.root) throw new TypeError('an archive root is required');
    this.clock = options.clock ?? (() => new Date());
  }

  async entries() {
    let names;
    try {
      names = await readdir(this.root, { withFileTypes: true });
    } catch (error) {
      if (error.code === 'ENOENT') return [];
      throw error;
    }
    return names
      .filter((entry) => entry.isFile() && /\.(json|csv)$/.test(entry.name))
      .map((entry) => ({
        name: entry.name,
        type: entry.name.endsWith('.json') ? 'application/json' : 'text/csv',
      }))
      .sort((left, right) => (left.name < right.name ? -1 : 1));
  }

  async write(name, rows) {
    if (!Array.isArray(rows)) throw new TypeError('archive rows must be an array');
    if (rows.length > MAX_ROWS) {
      throw new TypeError(`an archive entry holds at most ${MAX_ROWS} rows`);
    }
    const file = resolveUnder(this.root, assertPlainName(name));
    const body = name.endsWith('.json')
      ? JSON.stringify({ written_at: new Date(this.clock()).toISOString(), rows }, null, 2)
      : toCsv(rows);
    await writeFile(file, `${body}\n`);
    return { name, rows: rows.length, bytes: Buffer.byteLength(body) };
  }

  async read(name) {
    const clean = String(name ?? '');
    if (clean.length === 0) throw notFound('archive entry');
    const file = join(this.root, clean);
    const info = await stat(file).catch(() => undefined);
    if (!info || !info.isFile()) throw notFound(`archive entry ${name}`);
    const body = await readFile(file, 'utf8');
    if (clean.endsWith('.json')) {
      let parsed;
      try {
        parsed = JSON.parse(body);
      } catch (error) {
        throw new TypeError(`${clean} is not valid JSON: ${error.message}`);
      }
      return { name: clean, rows: parsed.rows, written_at: parsed.written_at ?? null };
    }
    return { name: clean, rows: fromCsv(body), written_at: null };
  }

  async remove(name) {
    const clean = assertPlainName(name);
    const file = resolveUnder(this.root, clean);
    const info = await stat(file).catch(() => undefined);
    if (!info) throw notFound(`archive entry ${name}`);
    await writeFile(file, '');
    return { name: clean, cleared: true };
  }

  async describe(name) {
    const clean = assertPlainName(name);
    const file = resolveUnder(this.root, clean);
    const info = await stat(file).catch(() => undefined);
    if (!info) throw notFound(`archive entry ${name}`);
    return { name: clean, bytes: info.size, modified: info.mtime.toISOString() };
  }
}

export function toCsv(rows) {
  if (rows.length === 0) return '';
  const fields = Object.keys(rows[0]);
  const lines = [fields.join(',')];
  for (const row of rows) {
    lines.push(fields.map((field) => cell(row[field])).join(','));
  }
  return lines.join('\n');
}

function cell(value) {
  const text = value === undefined || value === null ? '' : String(value);
  if (/[",\n]/.test(text)) return `"${text.replace(/"/g, '""')}"`;
  return text;
}

export function fromCsv(body) {
  const lines = String(body).split('\n').filter((line) => line.length > 0);
  if (lines.length === 0) return [];
  const fields = lines[0].split(',');
  return lines.slice(1).map((line) => {
    const values = line.split(',');
    const row = {};
    fields.forEach((field, index) => {
      row[field] = values[index] ?? '';
    });
    return row;
  });
}
