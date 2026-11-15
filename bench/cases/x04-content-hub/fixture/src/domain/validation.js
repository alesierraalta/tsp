import { coerceValue, defaultValue, fieldLabel } from './fields.js';

export const MAX_ENTRY_BYTES = 256 * 1024;

export function validateEntry(schema, values = {}, options = {}) {
  const errors = [];
  const cleaned = {};
  const partial = options.partial === true;
  for (const field of schema.fields ?? []) {
    if (field.system === true) continue;
    const present = Object.prototype.hasOwnProperty.call(values, field.name);
    if (!present) {
      if (field.required && !partial) {
        errors.push({ field: field.name, message: `${fieldLabel(field)} is required` });
      }
      continue;
    }
    const raw = values[field.name];
    if (raw === null || raw === undefined) {
      if (field.required) errors.push({ field: field.name, message: `${fieldLabel(field)} is required` });
      else cleaned[field.name] = defaultValue(field);
      continue;
    }
    const coerced = coerceValue(field, raw);
    if (coerced === undefined) {
      errors.push({ field: field.name, message: `${fieldLabel(field)} is not a valid ${field.type}` });
      continue;
    }
    const problem = checkFieldBounds(field, coerced);
    if (problem) {
      errors.push({ field: field.name, message: problem });
      continue;
    }
    cleaned[field.name] = coerced;
  }
  for (const key of Object.keys(values)) {
    if (schema.fields.some((field) => field.name === key)) continue;
    errors.push({ field: key, message: `${key} is not a field of ${schema.api_id}` });
  }
  return { ok: errors.length === 0, errors, values: cleaned };
}

function checkFieldBounds(field, value) {
  if (field.type === 'text' || field.type === 'long_text' || field.type === 'rich_text') {
    const max = field.max ?? 500;
    if (value.length > max) return `${fieldLabel(field)} is longer than ${max} characters`;
    if (field.min !== undefined && value.length < field.min) return `${fieldLabel(field)} is shorter than ${field.min}`;
    return undefined;
  }
  if (field.type === 'number' || field.type === 'integer') {
    if (field.min !== undefined && value < field.min) return `${fieldLabel(field)} is below ${field.min}`;
    if (field.max !== undefined && value > field.max) return `${fieldLabel(field)} is above ${field.max}`;
    return undefined;
  }
  if (field.type === 'date' && field.name.endsWith('_at')) {
    const instant = new Date(value);
    if (Number.isNaN(instant.getTime())) return `${fieldLabel(field)} is not a date`;
  }
  return undefined;
}

export function validatePartial(schema, values = {}) {
  return validateEntry(schema, values, { partial: true });
}

export function violationError(errors) {
  const first = errors[0];
  const error = new Error(first?.message ?? 'invalid entry');
  error.code = 'invalid_entry';
  error.status = 400;
  error.details = { errors };
  error.toJSON = () => ({ error: 'invalid_entry', message: first?.message ?? 'invalid entry', details: { errors } });
  return error;
}

export function requireValid(schema, values, options = {}) {
  const result = validateEntry(schema, values, options);
  if (!result.ok) throw violationError(result.errors);
  return result.values;
}

export function diffValues(before, after) {
  const changed = {};
  for (const [key, value] of Object.entries(after ?? {})) {
    const previous = before?.[key];
    if (JSON.stringify(previous ?? null) !== JSON.stringify(value ?? null)) changed[key] = value;
  }
  return changed;
}

export function sizeOfValues(values) {
  return Buffer.byteLength(JSON.stringify(values ?? {}), 'utf8');
}

export function assertSize(values, limit = MAX_ENTRY_BYTES) {
  const bytes = sizeOfValues(values);
  if (bytes > limit) throw new TypeError(`an entry holds at most ${limit} bytes, got ${bytes}`);
  return bytes;
}

export function normalizeStatus(status) {
  const text = String(status ?? '').toLowerCase();
  return ['draft', 'published', 'archived'].includes(text) ? text : 'draft';
}

const JSON_LIMIT_BYTES = 8 * 1024;

export function validateJsonField(field, value) {
  if (value === null || value === undefined) return { ok: true, value: null };
  let bytes;
  try {
    bytes = Buffer.byteLength(JSON.stringify(value), 'utf8');
  } catch {
    return { ok: false, message: `${fieldLabel(field)} is not serializable` };
  }
  if (bytes > (field.maxBytes ?? JSON_LIMIT_BYTES)) {
    return { ok: false, message: `${fieldLabel(field)} is larger than ${field.maxBytes ?? JSON_LIMIT_BYTES} bytes` };
  }
  if (field.shape === 'object' && (typeof value !== 'object' || Array.isArray(value))) {
    return { ok: false, message: `${fieldLabel(field)} must be an object` };
  }
  if (field.shape === 'list' && !Array.isArray(value)) {
    return { ok: false, message: `${fieldLabel(field)} must be a list` };
  }
  return { ok: true, value };
}

export function validateReferences(schema, values, exists) {
  const problems = [];
  for (const field of schema.fields ?? []) {
    if (field.type !== 'reference') continue;
    const value = values[field.name];
    if (value === undefined || value === null || value === '') continue;
    if (!exists(value)) problems.push({ field: field.name, message: `${fieldLabel(field)} points at a missing record` });
  }
  return problems;
}

export function errorSummary(errors) {
  if (!Array.isArray(errors) || errors.length === 0) return 'no errors';
  const fields = errors.map((error) => error.field ?? 'value');
  return `${errors.length} problem(s): ${[...new Set(fields)].join(', ')}`;
}

export function validateRequiredGroups(schema, values) {
  const groups = schema.required_groups ?? [];
  const problems = [];
  for (const group of groups) {
    const present = group.fields.filter((name) => {
      const value = values[name];
      return value !== undefined && value !== null && String(value).trim() !== '';
    });
    if (present.length < (group.at_least ?? 1)) {
      problems.push({ field: group.fields.join('|'), message: `at least ${group.at_least ?? 1} of ${group.fields.join(', ')} is required` });
    }
  }
  return problems;
}

export function mergeValidated(schema, current, patch, options = {}) {
  const check = validateEntry(schema, { ...current, ...patch }, { partial: true });
  if (!check.ok) throw violationError(check.errors);
  const problems = validateRequiredGroups(schema, check.values);
  if (problems.length > 0 && options.skipGroups !== true) throw violationError(problems);
  return check.values;
}

export function coerceNumbers(schema, values) {
  const out = { ...values };
  for (const field of schema.fields ?? []) {
    if (field.type !== 'number' && field.type !== 'integer') continue;
    const value = out[field.name];
    if (typeof value === 'string' && value.trim() !== '') {
      const number = Number(value);
      if (Number.isFinite(number)) out[field.name] = field.type === 'integer' ? Math.trunc(number) : number;
    }
  }
  return out;
}
