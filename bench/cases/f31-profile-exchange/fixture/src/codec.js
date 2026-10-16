import { checkFields, mutable, text } from './schema.js';

export function encodePatch(input) {
  checkFields(input, mutable);
  const result = {};
  for (const field of mutable) {
    if (input[field] !== undefined) result[field] = text(input[field], field);
  }
  return JSON.stringify(result);
}

export function decodePatch(body) {
  if (typeof body !== 'string') throw new TypeError('body must be text');
  const parsed = JSON.parse(body);
  checkFields(parsed, mutable);
  const result = {};
  for (const field of Object.keys(parsed)) {
    result[field] = text(parsed[field], field);
  }
  return result;
}

export function applyPatch(record, patch) {
  const result = { ...record };
  checkFields(patch, mutable);
  for (const field of Object.keys(patch)) {
    result[field] = text(patch[field], field);
  }
  return result;
}

export function encodeSnapshot(rows, revision) {
  return JSON.stringify({ revision, rows });
}

export function decodeSnapshot(body) {
  const value = JSON.parse(body);
  if (!Number.isSafeInteger(value.revision) || value.revision < 0) {
    throw new TypeError('invalid revision');
  }
  if (!Array.isArray(value.rows)) throw new TypeError('invalid rows');
  return value;
}
