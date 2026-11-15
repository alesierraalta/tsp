import { compareInstants } from '../util/time.js';

export const OPERATORS = ['eq', 'ne', 'lt', 'lte', 'gt', 'gte', 'in', 'contains', 'exists', 'starts'];

const UNSAFE_PART = new Set(['__proto__', 'constructor', 'prototype']);

export function readPath(row, path) {
  const parts = String(path).split('.');
  let value = row;
  for (const part of parts) {
    if (value === null || value === undefined || UNSAFE_PART.has(part)) return undefined;
    if (!Object.prototype.hasOwnProperty.call(Object(value), part)) return undefined;
    value = value[part];
  }
  return value;
}

export function compareValues(left, right) {
  if (left === right) return 0;
  if (left === undefined || left === null) return -1;
  if (right === undefined || right === null) return 1;
  if (typeof left === 'number' && typeof right === 'number') return left - right;
  if (left instanceof Date || right instanceof Date) return compareInstants(left, right);
  const a = String(left);
  const b = String(right);
  if (a === b) return 0;
  return a < b ? -1 : 1;
}

function matchesOperator(actual, operator, expected) {
  switch (operator) {
    case 'eq':
      return compareValues(actual, expected) === 0;
    case 'ne':
      return compareValues(actual, expected) !== 0;
    case 'lt':
      return compareValues(actual, expected) < 0;
    case 'lte':
      return compareValues(actual, expected) <= 0;
    case 'gt':
      return compareValues(actual, expected) > 0;
    case 'gte':
      return compareValues(actual, expected) >= 0;
    case 'in':
      return Array.isArray(expected) && expected.some((item) => compareValues(actual, item) === 0);
    case 'contains':
      return String(actual ?? '').toLowerCase().includes(String(expected ?? '').toLowerCase());
    case 'starts':
      return String(actual ?? '').toLowerCase().startsWith(String(expected ?? '').toLowerCase());
    case 'exists':
      return expected ? actual !== undefined && actual !== null : actual === undefined || actual === null;
    default:
      throw new TypeError(`unknown operator ${operator}`);
  }
}

export function matchesAll(row, filters = []) {
  return filters.every((filter) => matchesOperator(readPath(row, filter.path), filter.op ?? 'eq', filter.value));
}

export function normalizeFilters(input = {}) {
  const filters = [];
  for (const [path, value] of Object.entries(input)) {
    if (value === undefined || value === '') continue;
    filters.push({ path, op: 'eq', value });
  }
  return filters;
}

export function parseSort(spec, allowed = []) {
  const text = String(spec ?? '').trim();
  if (text === '') return [];
  return text
    .split(',')
    .map((part) => {
      const trimmed = part.trim();
      const descending = trimmed.startsWith('-');
      const path = descending ? trimmed.slice(1) : trimmed;
      if (allowed.length > 0 && !allowed.includes(path)) {
        throw new TypeError(`cannot sort by ${path}`);
      }
      return { path, direction: descending ? 'desc' : 'asc' };
    })
    .filter((term) => term.path !== '');
}

export function sortRows(rows, terms = []) {
  if (terms.length === 0) return rows;
  return [...rows].sort((left, right) => {
    for (const term of terms) {
      const order = compareValues(readPath(left, term.path), readPath(right, term.path));
      if (order !== 0) return term.direction === 'desc' ? -order : order;
    }
    return 0;
  });
}

export function applyQuery(rows, query = {}) {
  let out = Array.isArray(rows) ? rows : [...rows];
  if (query.where) out = out.filter((row) => matchesAll(row, query.where));
  if (query.search) {
    const needle = String(query.search).toLowerCase();
    const fields = query.searchFields ?? Object.keys(rows[0] ?? {});
    out = out.filter((row) => fields.some((field) => String(readPath(row, field) ?? '').toLowerCase().includes(needle)));
  }
  if (query.sort) out = sortRows(out, query.sort);
  const total = out.length;
  const offset = Math.max(0, Number(query.offset ?? 0));
  const limit = Number(query.limit ?? total);
  const page = out.slice(offset, offset + limit);
  return { rows: page, total, offset, limit, has_more: offset + page.length < total };
}

export function project(row, fields) {
  if (fields.length === 0) return row;
  const out = {};
  for (const field of fields) {
    const value = readPath(row, field);
    if (value !== undefined) out[field] = value;
  }
  return out;
}

export function countWhere(rows, filters = []) {
  return rows.filter((row) => matchesAll(row, filters)).length;
}

export function aggregate(rows, path, operation = 'count') {
  const values = rows.map((row) => readPath(row, path)).filter((value) => value !== undefined && value !== null);
  switch (operation) {
    case 'count':
      return values.length;
    case 'sum':
      return values.reduce((total, value) => total + (Number(value) || 0), 0);
    case 'avg':
      return values.length === 0 ? 0 : values.reduce((total, value) => total + (Number(value) || 0), 0) / values.length;
    case 'min':
      return values.reduce((best, value) => (best === undefined || compareValues(value, best) < 0 ? value : best), undefined);
    case 'max':
      return values.reduce((best, value) => (best === undefined || compareValues(value, best) > 0 ? value : best), undefined);
    case 'distinct':
      return new Set(values.map((value) => String(value))).size;
    default:
      throw new TypeError(`unknown aggregate ${operation}`);
  }
}

export function groupAggregate(rows, groupPath, valuePath, operation = 'count') {
  const groups = new Map();
  for (const row of rows) {
    const key = String(readPath(row, groupPath) ?? '');
    const bucket = groups.get(key);
    if (bucket) bucket.push(row);
    else groups.set(key, [row]);
  }
  const out = {};
  for (const [key, bucket] of groups.entries()) {
    out[key] = valuePath === undefined ? bucket.length : aggregate(bucket, valuePath, operation);
  }
  return out;
}

export function encodeCursor(row, fields = ['updated_at', 'id']) {
  const parts = fields.map((field) => String(readPath(row, field) ?? ''));
  return Buffer.from(parts.join('\u0000'), 'utf8').toString('base64url');
}

export function decodeCursor(cursor, fields = ['updated_at', 'id']) {
  if (typeof cursor !== 'string' || cursor === '') return undefined;
  const text = Buffer.from(cursor, 'base64url').toString('utf8');
  const parts = text.split('\u0000');
  if (parts.length !== fields.length) return undefined;
  return Object.fromEntries(fields.map((field, index) => [field, parts[index]]));
}

export function afterCursor(rows, cursor, fields = ['updated_at', 'id'], direction = 'desc') {
  if (!cursor) return rows;
  const sign = direction === 'desc' ? -1 : 1;
  return rows.filter((row) => {
    for (const field of fields) {
      const order = compareValues(readPath(row, field), cursor[field]);
      if (order === 0) continue;
      return order * sign > 0;
    }
    return false;
  });
}

export function selectFields(row, fields) {
  return fields.length === 0 ? row : project(row, fields);
}
