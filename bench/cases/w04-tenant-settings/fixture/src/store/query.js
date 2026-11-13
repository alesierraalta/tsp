import { dirname } from 'node:path';

const SCHEMA = {
  tenants: { fields: ['id', 'slug', 'name', 'plan', 'settings', 'created_at', 'updated_at'] },
  users: { fields: ['id', 'tenant_id', 'email', 'name', 'roles', 'settings', 'created_at', 'updated_at'] },
  audits: { fields: ['id', 'tenant_id', 'actor', 'action', 'target', 'at'] },
};

export function schemaFor(name) {
  const schema = SCHEMA[name];
  if (!schema) throw new TypeError(`no schema for ${name}`);
  return schema;
}

export function projectRow(name, row) {
  const { fields } = schemaFor(name);
  const out = {};
  for (const field of fields) {
    if (row[field] !== undefined) out[field] = row[field];
  }
  return out;
}

export function validateRow(name, row) {
  const { fields } = schemaFor(name);
  const unknown = Object.keys(row).filter((key) => !fields.includes(key));
  if (unknown.length > 0) {
    throw new TypeError(`unknown ${name} field: ${unknown.join(', ')}`);
  }
  return row;
}

export function applyFilters(rows, filters, schema) {
  const { fields } = schema;
  let out = rows;
  for (const [key, value] of Object.entries(filters ?? {})) {
    if (value === undefined || value === '') continue;
    if (key.endsWith('_min')) {
      const field = key.slice(0, -4);
      if (!fields.includes(field)) continue;
      out = out.filter((row) => row[field] !== undefined && row[field] >= value);
      continue;
    }
    if (key.endsWith('_max')) {
      const field = key.slice(0, -4);
      if (!fields.includes(field)) continue;
      out = out.filter((row) => row[field] !== undefined && row[field] <= value);
      continue;
    }
    if (!fields.includes(key)) continue;
    out = out.filter((row) => String(row[key] ?? '') === String(value));
  }
  return out;
}

export function sortRows(rows, options, schema) {
  const field = options.sort ?? 'created_at';
  if (!schema.fields.includes(field)) {
    throw new TypeError(`cannot sort by ${field}`);
  }
  const direction = options.dir === 'desc' ? -1 : 1;
  const copy = [...rows];
  copy.sort((left, right) => {
    const a = left[field] ?? '';
    const b = right[field] ?? '';
    if (a === b) return 0;
    return a < b ? -direction : direction;
  });
  return copy;
}

export const MAX_LIMIT = 100;

export function clampLimit(value, fallback = 25) {
  const parsed = Number(value);
  if (!Number.isFinite(parsed) || parsed <= 0) return fallback;
  return Math.min(Math.floor(parsed), MAX_LIMIT);
}

export function paginate(rows, options = {}) {
  const limit = clampLimit(options.limit);
  const offset = Math.max(0, Number(options.offset) || 0);
  const page = rows.slice(offset, offset + limit);
  return {
    rows: page,
    limit,
    offset,
    total: rows.length,
    next: offset + limit < rows.length ? String(offset + limit) : null,
  };
}

export function projectFields(rows, requested, schema) {
  const fields = (requested ?? schema.fields).filter((field) => schema.fields.includes(field));
  const chosen = fields.length > 0 ? fields : schema.fields;
  return rows.map((row) => {
    const out = {};
    for (const field of chosen) {
      if (row[field] !== undefined) out[field] = row[field];
    }
    return out;
  });
}

export function groupBy(rows, field) {
  const groups = new Map();
  for (const row of rows) {
    const key = String(row[field] ?? '');
    if (!groups.has(key)) groups.set(key, []);
    groups.get(key).push(row);
  }
  return groups;
}

export function countsBy(rows, field) {
  const out = Object.create(null);
  for (const row of rows) {
    const key = String(row[field] ?? 'none');
    out[key] = (out[key] ?? 0) + 1;
  }
  return out;
}

export function sortByKey(map, direction = 'asc') {
  const entries = [...map.entries()];
  entries.sort((left, right) => (left[0] < right[0] ? -1 : left[0] > right[0] ? 1 : 0));
  return direction === 'desc' ? entries.reverse() : entries;
}

export function parentOf(path) {
  return dirname(path);
}
