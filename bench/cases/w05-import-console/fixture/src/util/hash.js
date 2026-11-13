import { createHash } from 'node:crypto';

export function fingerprint(parts) {
  const payload = parts.map((part) => String(part ?? '')).join('\u0000');
  return createHash('sha256').update(payload).digest('hex').slice(0, 24);
}

export function shortHash(value, length = 8) {
  return createHash('sha256').update(String(value ?? '')).digest('hex').slice(0, length);
}

export function stableStringify(value) {
  if (value === null || typeof value !== 'object') return JSON.stringify(value);
  if (Array.isArray(value)) return `[${value.map(stableStringify).join(',')}]`;
  const keys = Object.keys(value).sort();
  return `{${keys.map((key) => `${JSON.stringify(key)}:${stableStringify(value[key])}`).join(',')}}`;
}

export function checksumOf(rows) {
  return fingerprint(rows.map((row) => stableStringify(row)));
}
