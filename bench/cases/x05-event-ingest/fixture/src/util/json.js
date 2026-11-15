export function clone(value) {
  if (value === null || typeof value !== 'object') return value;
  if (Array.isArray(value)) return value.map((item) => clone(item));
  if (value instanceof Date) return new Date(value.getTime());
  const out = {};
  for (const [key, item] of Object.entries(value)) out[key] = clone(item);
  return out;
}

export const UNSAFE_KEYS = new Set(['__proto__', 'constructor', 'prototype']);

export function hasUnsafeKey(value) {
  if (value === null || typeof value !== 'object') return false;
  for (const [key, item] of Object.entries(value)) {
    if (UNSAFE_KEYS.has(key)) return true;
    if (hasUnsafeKey(item)) return true;
  }
  return false;
}

export function stableStringify(value, indent = 0) {
  const pad = ' '.repeat(indent);
  const inner = ' '.repeat(indent + 2);
  if (value === null || typeof value !== 'object') return JSON.stringify(value);
  if (Array.isArray(value)) {
    if (value.length === 0) return '[]';
    return `[\n${value.map((item) => `${inner}${stableStringify(item, indent + 2)}`).join(',\n')}\n${pad}]`;
  }
  const keys = Object.keys(value).filter((key) => value[key] !== undefined).sort();
  if (keys.length === 0) return '{}';
  return `{\n${keys.map((key) => `${inner}${JSON.stringify(key)}: ${stableStringify(value[key], indent + 2)}`).join(',\n')}\n${pad}}`;
}

export function parseJsonOrThrow(text, label = 'payload') {
  try {
    return JSON.parse(text);
  } catch (error) {
    throw new TypeError(`${label} is not valid JSON: ${error.message}`);
  }
}

export function parseSafe(text, fallback = undefined) {
  try {
    return typeof text === 'string' ? JSON.parse(text) : fallback;
  } catch {
    return fallback;
  }
}

export function pick(value, keys) {
  const out = {};
  for (const key of keys) {
    if (value && Object.prototype.hasOwnProperty.call(value, key)) out[key] = value[key];
  }
  return out;
}

export function omit(value, keys) {
  const skip = new Set(keys);
  return Object.fromEntries(Object.entries(value ?? {}).filter(([key]) => !skip.has(key)));
}

export function flatten(value, prefix = '', out = {}) {
  for (const [key, item] of Object.entries(value ?? {})) {
    const path = prefix === '' ? key : `${prefix}.${key}`;
    if (item && typeof item === 'object' && !Array.isArray(item)) flatten(item, path, out);
    else out[path] = item;
  }
  return out;
}

export function byteSize(value) {
  return Buffer.byteLength(typeof value === 'string' ? value : JSON.stringify(value ?? null), 'utf8');
}

export function deepEqual(left, right) {
  if (left === right) return true;
  if (left === null || right === null || typeof left !== 'object' || typeof right !== 'object') return false;
  if (Array.isArray(left) !== Array.isArray(right)) return false;
  const leftKeys = Object.keys(left);
  if (leftKeys.length !== Object.keys(right).length) return false;
  return leftKeys.every((key) => deepEqual(left[key], right[key]));
}
