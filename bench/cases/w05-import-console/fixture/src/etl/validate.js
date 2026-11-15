import { problem } from '../util/errors.js';
import { isDateString } from '../util/time.js';
import { LIMITS } from '../config.js';
import { isEmail } from './schema.js';

function checkCell(column, value) {
  const text = value === null || value === undefined ? '' : String(value);
  if (column.required && text.trim() === '') {
    return problem(column.name, 'is required');
  }
  if (text.length > column.maxLength) {
    return problem(column.name, `is longer than ${column.maxLength} characters`);
  }
  if (text.trim() === '') return undefined;
  switch (column.type) {
    case 'integer':
      if (!/^-?\d+$/.test(text.trim())) return problem(column.name, 'is not a whole number');
      return undefined;
    case 'money':
      if (!/^-?\d+(\.\d{1,4})?$/.test(text.trim().replace(/[,$\s]/g, ''))) {
        return problem(column.name, 'is not a money amount');
      }
      return undefined;
    case 'date':
      if (!isDateString(text.trim())) return problem(column.name, 'is not an ISO date');
      return undefined;
    case 'enum':
      if (!column.values.includes(text.trim().toLowerCase())) {
        return problem(column.name, `must be one of ${column.values.join(', ')}`);
      }
      return undefined;
    case 'email':
      if (!isEmail(text.trim())) return problem(column.name, 'is not an email address');
      return undefined;
    default:
      return undefined;
  }
}

export function validateRow(schema, row, rowNumber) {
  const problems = [];
  for (const column of schema.columns) {
    const value = row[column.name];
    if (value !== undefined && String(value).length > LIMITS.maxCellLength) {
      problems.push(problem(column.name, `is longer than ${LIMITS.maxCellLength} characters`));
      continue;
    }
    const found = checkCell(column, value);
    if (found) problems.push(found);
  }
  const known = new Set(schema.columns.map((column) => column.name));
  const unknown = Object.keys(row).filter((name) => !known.has(name));
  return { row: rowNumber, problems, unknown };
}

export function validateRows(schema, rows, options = {}) {
  if (!Array.isArray(rows)) throw new TypeError('rows must be a list');
  const limit = options.limit ?? LIMITS.maxRows;
  const results = rows.map((row, index) => validateRow(schema, row, index + 2));
  const errors = results.filter((entry) => entry.problems.length > 0);
  const unknown = results.flatMap((entry) => entry.unknown.map((name) => ({ row: entry.row, name })));
  return {
    ok: errors.length === 0 && (options.allowUnknown || unknown.length === 0),
    rows: results.length,
    errors,
    unknown,
    truncated: rows.length > limit,
  };
}

export function errorReport(validation, options = {}) {
  const lines = [`${validation.rows} rows checked`, `${validation.errors.length} rows with problems`];
  for (const entry of validation.errors.slice(0, options.limit ?? 20)) {
    lines.push(`row ${entry.row}: ${entry.problems.map((item) => `${item.field} ${item.message}`).join(', ')}`);
  }
  if (validation.unknown.length > 0) {
    lines.push(`unknown columns: ${[...new Set(validation.unknown.map((entry) => entry.name))].join(', ')}`);
  }
  return lines.join('\n');
}

export function firstProblem(validation) {
  const entry = validation.errors[0];
  if (!entry) return undefined;
  return entry.problems[0];
}

export function countProblems(validation) {
  return validation.errors.reduce((sum, entry) => sum + entry.problems.length, 0);
}

export function problemsByColumn(validation) {
  const counts = Object.create(null);
  for (const entry of validation.errors) {
    for (const item of entry.problems) {
      counts[item.field] = (counts[item.field] ?? 0) + 1;
    }
  }
  return counts;
}
