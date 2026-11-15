import { randomBytes } from 'node:crypto';

const ALPHABET = '0123456789abcdefghjkmnpqrstvwxyz';

export function randomToken(size = 10) {
  const bytes = randomBytes(size);
  let out = '';
  for (let index = 0; index < bytes.length; index += 1) {
    out += ALPHABET[bytes[index] % ALPHABET.length];
  }
  return out;
}

export function newId(prefix) {
  if (typeof prefix !== 'string' || prefix.length === 0) {
    throw new TypeError('an id prefix is required');
  }
  return `${prefix}_${randomToken(8)}`;
}

export function isId(value, prefix) {
  if (typeof value !== 'string') return false;
  const parts = value.split('_');
  if (parts.length !== 2) return false;
  if (prefix !== undefined && parts[0] !== prefix) return false;
  return /^[0-9a-hjkmnp-tv-z]{8}$/.test(parts[1]);
}

export function rowKey(row, fields) {
  return fields.map((field) => String(row[field] ?? '')).join('\u0000');
}

export function sequence(prefix, count) {
  const out = [];
  for (let index = 1; index <= count; index += 1) {
    out.push(`${prefix}${String(index).padStart(4, '0')}`);
  }
  return out;
}
