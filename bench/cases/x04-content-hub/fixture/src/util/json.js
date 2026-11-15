export function clone(value) {
  if (value === null || typeof value !== 'object') return value;
  if (Array.isArray(value)) return value.map((item) => clone(item));
  if (value instanceof Date) return new Date(value.getTime());
  const out = {};
  for (const [key, item] of Object.entries(value)) out[key] = clone(item);
  return out;
}

export function stableStringify(value, indent = 0) {
  const pad = ' '.repeat(indent);
  const inner = ' '.repeat(indent + 2);
  if (value === null || typeof value !== 'object') return JSON.stringify(value);
  if (value instanceof Date) return JSON.stringify(value.toISOString());
  if (Array.isArray(value)) {
    if (value.length === 0) return '[]';
    const body = value.map((item) => `${inner}${stableStringify(item, indent + 2)}`).join(',\n');
    return `[\n${body}\n${pad}]`;
  }
  const keys = Object.keys(value).filter((key) => value[key] !== undefined).sort();
  if (keys.length === 0) return '{}';
  const body = keys
    .map((key) => `${inner}${JSON.stringify(key)}: ${stableStringify(value[key], indent + 2)}`)
    .join(',\n');
  return `{\n${body}\n${pad}}`;
}

export function parseSafe(text, fallback = undefined) {
  if (typeof text !== 'string') return fallback;
  try {
    return JSON.parse(text);
  } catch {
    return fallback;
  }
}

export function parseJsonOrThrow(text, label = 'document') {
  try {
    return JSON.parse(text);
  } catch (error) {
    throw new TypeError(`${label} is not valid JSON: ${error.message}`);
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
  const out = {};
  const skip = new Set(keys);
  for (const [key, item] of Object.entries(value ?? {})) {
    if (!skip.has(key)) out[key] = item;
  }
  return out;
}

export function deepEqual(left, right) {
  if (left === right) return true;
  if (left === null || right === null) return false;
  if (typeof left !== 'object' || typeof right !== 'object') return false;
  if (Array.isArray(left) !== Array.isArray(right)) return false;
  const leftKeys = Object.keys(left);
  const rightKeys = Object.keys(right);
  if (leftKeys.length !== rightKeys.length) return false;
  return leftKeys.every((key) => deepEqual(left[key], right[key]));
}

export function isEmpty(value) {
  if (value === null || value === undefined) return true;
  if (typeof value === 'string' || Array.isArray(value)) return value.length === 0;
  if (typeof value === 'object') return Object.keys(value).length === 0;
  return false;
}

export function flatten(value, prefix = '', out = {}) {
  for (const [key, item] of Object.entries(value ?? {})) {
    const path = prefix ? `${prefix}.${key}` : key;
    if (item && typeof item === 'object' && !Array.isArray(item) && !(item instanceof Date)) {
      flatten(item, path, out);
    } else {
      out[path] = item;
    }
  }
  return out;
}

export function sizeOf(value) {
  return Buffer.byteLength(JSON.stringify(value ?? null));
}

export const UNSAFE_KEYS = new Set(['__proto__', 'constructor', 'prototype']);

export function hasUnsafeKey(value, seen = new Set()) {
  if (value === null || typeof value !== 'object') return false;
  if (seen.has(value)) return false;
  seen.add(value);
  for (const [key, item] of Object.entries(value)) {
    if (UNSAFE_KEYS.has(key)) return true;
    if (hasUnsafeKey(item, seen)) return true;
  }
  return false;
}

export function assertSafeKeys(value) {
  if (hasUnsafeKey(value)) throw new TypeError('a payload may not carry __proto__, constructor or prototype keys');
  return value;
}

export function mergeSafe(base, patch, depth = 0) {
  if (depth > 8) throw new TypeError('a merge nests more than 8 levels deep');
  const out = { ...clone(base ?? {}) };
  for (const [key, value] of Object.entries(patch ?? {})) {
    if (UNSAFE_KEYS.has(key)) continue;
    const current = out[key];
    if (value && typeof value === 'object' && !Array.isArray(value) && current && typeof current === 'object') {
      out[key] = mergeSafe(current, value, depth + 1);
      continue;
    }
    out[key] = clone(value);
  }
  return out;
}

export function patchPaths(value, prefix = '', out = []) {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) {
    if (prefix !== '') out.push(prefix);
    return out;
  }
  for (const [key, item] of Object.entries(value)) {
    patchPaths(item, prefix === '' ? key : `${prefix}.${key}`, out);
  }
  return out;
}

export function valueAt(value, path) {
  return String(path)
    .split('.')
    .reduce((acc, part) => (acc === null || acc === undefined ? undefined : acc[part]), value);
}

export function setAt(value, path, next) {
  const parts = String(path).split('.');
  const out = clone(value ?? {});
  let cursor = out;
  for (const part of parts.slice(0, -1)) {
    if (UNSAFE_KEYS.has(part)) return out;
    if (cursor[part] === null || typeof cursor[part] !== 'object') cursor[part] = {};
    cursor = cursor[part];
  }
  const leaf = parts[parts.length - 1];
  if (!UNSAFE_KEYS.has(leaf)) cursor[leaf] = next;
  return out;
}

export function byteSizeOf(value) {
  return Buffer.byteLength(typeof value === 'string' ? value : JSON.stringify(value ?? null), 'utf8');
}
