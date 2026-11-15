import { parseCents } from '../money/decimal.js';
import { canonicalHeader } from '../util/text.js';
import { coerce } from './schema.js';

export function canonicalHeaders(header) {
  const seen = new Map();
  return header.map((name) => {
    let key = canonicalHeader(name);
    if (key.length === 0) key = 'column';
    if (seen.has(key)) {
      const count = seen.get(key) + 1;
      seen.set(key, count);
      key = `${key}_${count}`;
    } else {
      seen.set(key, 0);
    }
    return key;
  });
}

export function renameColumns(rows, header, schema) {
  const canonical = canonicalHeaders(header);
  const allowed = new Set(schema.columns.map((column) => column.name));
  return rows.map((row) => {
    const out = {};
    header.forEach((original, index) => {
      const name = canonical[index];
      if (allowed.has(name)) out[name] = row[original];
    });
    return out;
  });
}

export function blankToNull(row) {
  const out = {};
  for (const [key, value] of Object.entries(row)) {
    out[key] = typeof value === 'string' && value.trim() === '' ? null : value;
  }
  return out;
}

export function normalizeRow(schema, row) {
  const out = {};
  for (const column of schema.columns) {
    const raw = row[column.name];
    if (raw === undefined) {
      if (column.default !== undefined) out[column.name] = column.default;
      continue;
    }
    if (typeof raw === 'string') out[column.name] = coerce(column.type, raw);
    else out[column.name] = raw;
  }
  return out;
}

export function normalizeRows(schema, rows) {
  return rows.map((row) => normalizeRow(schema, blankToNull(row)));
}

export function centsColumns(schema) {
  return schema.columns.filter((column) => column.type === 'money').map((column) => column.name);
}

export function toCents(row, columns) {
  const out = { ...row };
  for (const name of columns) {
    if (out[name] === undefined || out[name] === null) continue;
    out[name] = parseCents(out[name]);
  }
  return out;
}

export function normalizeMoney(schema, rows) {
  const columns = centsColumns(schema);
  return rows.map((row) => toCents(row, columns));
}

export function normalizeEmail(value) {
  return String(value ?? '').normalize('NFKC').trim().toLowerCase();
}

export function normalizeEnum(value) {
  return String(value ?? '').trim().toLowerCase();
}

export function mergeDuplicate(rows, keyFields, combine = (left) => left) {
  const index = new Map();
  const order = [];
  for (const row of rows) {
    const key = keyFields.map((field) => String(row[field] ?? '').toLowerCase()).join('\u0000');
    if (!index.has(key)) {
      index.set(key, row);
      order.push(key);
      continue;
    }
    index.set(key, combine(index.get(key), row));
  }
  return order.map((key) => index.get(key));
}
