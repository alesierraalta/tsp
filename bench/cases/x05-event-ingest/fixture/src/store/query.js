import { toInstant } from '../util/time.js';

const UNSAFE = new Set(['__proto__', 'constructor', 'prototype']);

export function readPath(row, path) {
  const parts = String(path).split('.');
  let value = row;
  for (const part of parts) {
    if (value === null || value === undefined || UNSAFE.has(part)) return undefined;
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
  const a = String(left);
  const b = String(right);
  return a === b ? 0 : a < b ? -1 : 1;
}

export function matches(row, filters = []) {
  return filters.every((filter) => {
    const actual = readPath(row, filter.path);
    switch (filter.op ?? 'eq') {
      case 'eq':
        return compareValues(actual, filter.value) === 0;
      case 'ne':
        return compareValues(actual, filter.value) !== 0;
      case 'gt':
        return compareValues(actual, filter.value) > 0;
      case 'gte':
        return compareValues(actual, filter.value) >= 0;
      case 'lt':
        return compareValues(actual, filter.value) < 0;
      case 'lte':
        return compareValues(actual, filter.value) <= 0;
      case 'in':
        return Array.isArray(filter.value) && filter.value.some((value) => compareValues(actual, value) === 0);
      case 'after':
        return (toInstant(actual)?.getTime() ?? 0) > (toInstant(filter.value)?.getTime() ?? 0);
      case 'before':
        return (toInstant(actual)?.getTime() ?? 0) < (toInstant(filter.value)?.getTime() ?? 0);
      default:
        throw new TypeError(`unknown operator ${filter.op}`);
    }
  });
}

export function sortRows(rows, terms = []) {
  return [...rows].sort((left, right) => {
    for (const term of terms) {
      const order = compareValues(readPath(left, term.path), readPath(right, term.path));
      if (order !== 0) return term.direction === 'desc' ? -order : order;
    }
    return 0;
  });
}

export function parseSort(spec) {
  return String(spec ?? '')
    .split(',')
    .map((part) => part.trim())
    .filter(Boolean)
    .map((part) => (part.startsWith('-') ? { path: part.slice(1), direction: 'desc' } : { path: part, direction: 'asc' }));
}

export function queryRows(rows, query = {}) {
  let out = [...(rows ?? [])];
  if (query.where) out = out.filter((row) => matches(row, query.where));
  if (query.sort) out = sortRows(out, parseSort(query.sort));
  const offset = Math.max(0, Number(query.offset ?? 0));
  const limit = Math.max(1, Math.min(500, Number(query.limit ?? 50)));
  const page = out.slice(offset, offset + limit);
  return { rows: page, total: out.length, offset, limit, has_more: offset + page.length < out.length };
}

export function aggregate(rows, path, operation = 'count') {
  const values = (rows ?? []).map((row) => readPath(row, path)).filter((value) => value !== undefined && value !== null);
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
    default:
      throw new TypeError(`unknown aggregate ${operation}`);
  }
}

export function groupCounts(rows, path) {
  const out = new Map();
  for (const row of rows ?? []) {
    const key = String(readPath(row, path) ?? '');
    out.set(key, (out.get(key) ?? 0) + 1);
  }
  return [...out.entries()].map(([value, count]) => ({ value, count })).sort((left, right) => right.count - left.count);
}

// A projection is what a caller may see of a row: only the paths it asked for, under the column names it
// asked for. Rows keep their identity through an `id` column when the spec asks for one.
export function project(row, paths) {
  const out = {};
  for (const path of paths ?? []) {
    const value = readPath(row, path);
    if (value !== undefined) out[String(path).replace(/\./g, '_')] = value;
  }
  return out;
}

export function projectRows(rows, paths) {
  return (rows ?? []).map((row) => project(row, paths));
}

export function countRows(rows, filters = []) {
  return (rows ?? []).filter((row) => matches(row, filters)).length;
}

export function distinctValues(rows, path, limit = 50) {
  const seen = new Set();
  for (const row of rows ?? []) {
    const value = readPath(row, path);
    if (value === undefined || value === null) continue;
    seen.add(value);
    if (seen.size >= limit) break;
  }
  return [...seen];
}

export function windowRows(rows, from, to, path = 'at') {
  return (rows ?? []).filter((row) => matches(row, [{ path, op: 'after', value: from }, { path, op: 'before', value: to }]));
}

export function describeQuery(spec = {}) {
  const filters = spec.where ?? [];
  const sort = parseSort(spec.sort);
  return {
    filters: filters.length,
    paths: filters.map((filter) => filter.path),
    sort: sort.map((term) => `${term.direction === 'desc' ? '-' : ''}${term.path}`),
    limit: Math.max(1, Math.min(500, Number(spec.limit ?? 50))),
  };
}

// explainQuery runs the query twice: once for the rows it returns and once to say how many rows the filters
// matched before the page was taken, so a caller can tell an empty page from an empty collection.
export function explainQuery(rows, spec = {}) {
  const filtered = (rows ?? []).filter((row) => matches(row, spec.where ?? []));
  const result = queryRows(filtered, spec);
  return {
    matched: filtered.length,
    returned: result.rows.length,
    offset: result.offset,
    limit: result.limit,
    has_more: result.has_more,
    description: describeQuery(spec),
  };
}

export function aggregateBy(rows, groupPath, valuePath, operation = 'count') {
  const groups = new Map();
  for (const row of rows ?? []) {
    const key = String(readPath(row, groupPath) ?? '');
    const bucket = groups.get(key) ?? [];
    bucket.push(row);
    groups.set(key, bucket);
  }
  return [...groups.entries()]
    .map(([key, bucket]) => ({ key, value: aggregate(bucket, valuePath, operation), rows: bucket.length }))
    .sort((left, right) => right.rows - left.rows);
}

export function compareRows(left, right, path) {
  return compareValues(readPath(left, path), readPath(right, path));
}
