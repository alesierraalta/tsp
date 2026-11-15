export function parseJson(text, fallback) {
  try {
    return JSON.parse(text);
  } catch {
    return fallback;
  }
}

export function canonicalJson(value) {
  if (value === null || typeof value !== 'object') return JSON.stringify(value);
  if (Array.isArray(value)) return `[${value.map(canonicalJson).join(',')}]`;
  const keys = Object.keys(value).sort();
  return `{${keys.map((key) => `${JSON.stringify(key)}:${canonicalJson(value[key])}`).join(',')}}`;
}

export function redactJson(value, keys = ['secret', 'token', 'password', 'authorization']) {
  if (Array.isArray(value)) return value.map((entry) => redactJson(entry, keys));
  if (value === null || typeof value !== 'object') return value;
  const out = {};
  for (const [key, entry] of Object.entries(value)) {
    out[key] = keys.includes(key.toLowerCase()) ? '[redacted]' : redactJson(entry, keys);
  }
  return out;
}

export function truncateJson(value, maxBytes = 4096) {
  const text = JSON.stringify(value ?? null);
  if (Buffer.byteLength(text) <= maxBytes) return text;
  return `${text.slice(0, Math.max(0, maxBytes - 20))}...[truncated]`;
}

export function jsonSize(value) {
  return Buffer.byteLength(JSON.stringify(value ?? null));
}

export function mergeJson(base, patch) {
  if (patch === null || typeof patch !== 'object' || Array.isArray(patch)) return patch;
  const out = { ...(base ?? {}) };
  for (const [key, value] of Object.entries(patch)) {
    out[key] = value !== null && typeof value === 'object' && !Array.isArray(value) ? mergeJson(out[key], value) : value;
  }
  return out;
}

export function pickJson(value, keys) {
  const out = {};
  for (const key of keys) if (Object.hasOwn(value ?? {}, key)) out[key] = value[key];
  return out;
}

export function jsonLines(values) {
  return values.map((value) => JSON.stringify(value)).join('\n');
}

export function jsonPath(value, path) {
  if (typeof path !== 'string' || path.length === 0) return value;
  let cursor = value;
  for (const part of path.split('.')) {
    if (cursor === null || typeof cursor !== 'object') return undefined;
    cursor = cursor[part];
  }
  return cursor;
}

export function deepKeys(value, prefix = '') {
  if (value === null || typeof value !== 'object') return [];
  const keys = [];
  for (const [key, entry] of Object.entries(value)) {
    const name = prefix === '' ? key : `${prefix}.${key}`;
    keys.push(name);
    keys.push(...deepKeys(entry, name));
  }
  return keys;
}

export function sizeReport(value) {
  return {
    bytes: jsonSize(value),
    keys: deepKeys(value).length,
    depth: Math.max(0, ...deepKeys(value).map((key) => key.split('.').length)),
  };
}

export function isJsonObject(value) {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

export function asArray(value) {
  if (Array.isArray(value)) return value;
  if (value === undefined || value === null) return [];
  return [value];
}
