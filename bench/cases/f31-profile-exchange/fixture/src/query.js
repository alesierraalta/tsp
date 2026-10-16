import { selection, fields } from './schema.js';

export function filterRows(rows, filter = {}) {
  for (const key of Object.keys(filter)) {
    if (!fields.includes(key)) throw new TypeError(`unknown filter: ${key}`);
  }
  return rows.filter(row => Object.entries(filter).every(([key, value]) => {
    return row[key] === value;
  }));
}

export function sortRows(rows, field = 'id', direction = 'asc') {
  selection([field]);
  if (!['asc', 'desc'].includes(direction)) throw new TypeError('invalid direction');
  const sign = direction === 'asc' ? 1 : -1;
  return [...rows].sort((a, b) => {
    const left = a[field] ?? '';
    const right = b[field] ?? '';
    if (left === right) return 0;
    return left < right ? -sign : sign;
  });
}

export function pageRows(rows, offset = 0, limit = 50) {
  if (!Number.isSafeInteger(offset) || offset < 0) throw new TypeError('invalid offset');
  if (!Number.isSafeInteger(limit) || limit < 1) throw new TypeError('invalid limit');
  return {
    rows: rows.slice(offset, offset + limit),
    total: rows.length,
    next: offset + limit < rows.length ? offset + limit : null,
  };
}

export function projectRows(rows, columns) {
  const chosen = selection(columns);
  return rows.map(row => Object.fromEntries(chosen.map(key => [key, row[key]])));
}

export function queryRows(rows, options = {}) {
  const filtered = filterRows(rows, options.filter);
  const sorted = sortRows(filtered, options.sort, options.direction);
  return pageRows(sorted, options.offset, options.limit);
}
