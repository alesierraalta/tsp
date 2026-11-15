import { parseCsv } from './csv.js';
import { parseJsonOrThrow } from '../util/json.js';
import { requireValid } from '../domain/validation.js';

export function detectFormat(name) {
  const text = String(name ?? '').toLowerCase();
  if (text.endsWith('.csv')) return 'csv';
  if (text.endsWith('.ndjson')) return 'ndjson';
  if (text.endsWith('.json')) return 'json';
  throw new TypeError(`cannot import ${name}: unknown format`);
}

export function recordsFrom(text, format) {
  switch (format) {
    case 'csv':
      return parseCsv(text).rows;
    case 'ndjson':
      return String(text)
        .split('\n')
        .filter((line) => line.trim() !== '')
        .map((line, index) => {
          try {
            return JSON.parse(line);
          } catch (error) {
            throw new TypeError(`import line ${index + 1} is not valid JSON: ${error.message}`);
          }
        });
    case 'json': {
      const parsed = parseJsonOrThrow(text, 'import');
      if (Array.isArray(parsed)) return parsed;
      if (Array.isArray(parsed.rows)) return parsed.rows;
      throw new TypeError('a json import needs a list of rows');
    }
    default:
      throw new TypeError(`unknown import format ${format}`);
  }
}

export class Importer {
  constructor(options = {}) {
    this.entries = options.entries;
    this.schemas = options.schemas;
    this.report = [];
  }

  importRecords(schema, rows, options = {}) {
    const created = [];
    const skipped = [];
    for (const row of rows) {
      const values = {};
      for (const field of schema.fields) {
        if (field.system === true) continue;
        if (row[field.name] !== undefined && row[field.name] !== '') values[field.name] = row[field.name];
      }
      try {
        const entry = this.entries.create(schema, { values }, {
          locale: row.locale ?? options.locale ?? 'en',
          status: options.status ?? 'draft',
          actor: options.actor ?? 'import',
        });
        created.push(entry);
      } catch (error) {
        if (options.lenient === true) {
          skipped.push({ slug: row.slug ?? values.slug ?? '', reason: error.message });
          continue;
        }
        throw error;
      }
    }
    this.report.push({ api_id: schema.api_id, created: created.length, skipped: skipped.length });
    return { created: created.length, skipped, entries: created };
  }

  importText(schema, name, text, options = {}) {
    const format = detectFormat(name);
    return this.importRecords(schema, recordsFrom(text, format), options);
  }

  validateOnly(schema, rows) {
    const problems = [];
    rows.forEach((row, index) => {
      const check = requireValidSafe(schema, row);
      if (!check.ok) problems.push({ row: index, errors: check.errors });
    });
    return problems;
  }

  summary() {
    return {
      files: this.report.length,
      created: this.report.reduce((total, item) => total + item.created, 0),
      skipped: this.report.reduce((total, item) => total + item.skipped, 0),
    };
  }
}

function requireValidSafe(schema, values) {
  try {
    return { ok: true, values: requireValid(schema, values) };
  } catch (error) {
    return { ok: false, errors: error.details?.errors ?? [{ message: error.message }] };
  }
}
