// Decodes and merges the untrusted options envelope a plugin sends with each request.
export const MaxBytes = 64 * 1024;

export class EnvelopeError extends Error {
  constructor(reason) {
    super(`envelope: ${reason}`);
    this.name = 'EnvelopeError';
  }
}

const KNOWN = ['timeoutMs', 'retries', 'endpoint', 'headers', 'logLevel'];

export function isPlainObject(value) {
  if (value === null || typeof value !== 'object' || Array.isArray(value)) return false;
  const proto = Object.getPrototypeOf(value);
  return proto === Object.prototype || proto === null;
}

export function decode(text) {
  if (typeof text !== 'string' || text.trim() === '') throw new EnvelopeError('empty body');
  if (text.length > MaxBytes) throw new EnvelopeError(`body is ${text.length} bytes, limit ${MaxBytes}`);
  let parsed;
  try {
    parsed = JSON.parse(text);
  } catch {
    throw new EnvelopeError('body is not json');
  }
  if (!isPlainObject(parsed)) throw new EnvelopeError('body must be a json object');
  return parsed;
}

export function merge(base, patch) {
  if (!isPlainObject(base) || !isPlainObject(patch)) throw new EnvelopeError('merge needs two objects');
  for (const [key, value] of Object.entries(patch)) {
    const current = Object.hasOwn(base, key) ? base[key] : undefined;
    if (isPlainObject(value) && isPlainObject(current)) {
      merge(current, value);
      continue;
    }
    setOption(base, key, isPlainObject(value) ? merge({}, value) : value);
  }
  return base;
}

const FORBIDDEN_KEYS = new Set(['__proto__', 'constructor', 'prototype']);

// setOption is the only place a merged option is written: a key that names a prototype slot is refused before
// the assignment, so an envelope can never reach Object.prototype.
function setOption(target, key, value) {
  if (FORBIDDEN_KEYS.has(key)) throw new EnvelopeError(`option ${key} is not allowed`);
  target[key] = value;
}

export function applyEnvelope(text, defaults = {}) {
  const envelope = decode(text);
  return merge(structuredClone(defaults), envelope);
}

export function pick(envelope, names = KNOWN) {
  if (!isPlainObject(envelope)) throw new EnvelopeError('pick needs an object');
  const wanted = new Set(names);
  const out = {};
  for (const key of Object.keys(envelope)) {
    if (wanted.has(key)) out[key] = envelope[key];
  }
  return out;
}

export function headerMap(lines) {
  if (!Array.isArray(lines)) throw new EnvelopeError('headers must be a list of lines');
  const out = {};
  for (const line of lines) {
    const text = String(line ?? '');
    const index = text.indexOf(':');
    if (index <= 0) throw new EnvelopeError(`malformed header line: ${text}`);
    const name = text.slice(0, index).trim().toLowerCase();
    const value = text.slice(index + 1).trim();
    if (name === '') throw new EnvelopeError(`header without a name: ${text}`);
    out[name] = value;
  }
  return out;
}

export function coerceNumbers(options, names) {
  const out = { ...options };
  for (const name of names) {
    if (!Object.hasOwn(out, name)) continue;
    const value = Number(out[name]);
    if (!Number.isFinite(value) || value < 0) throw new EnvelopeError(`${name} must be a non-negative number`);
    out[name] = value;
  }
  return out;
}
