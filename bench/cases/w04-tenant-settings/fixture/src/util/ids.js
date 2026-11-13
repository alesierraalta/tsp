import { createHash, randomBytes } from 'node:crypto';

const ALPHABET = '0123456789abcdefghjkmnpqrstvwxyz';

export function randomToken(size = 12) {
  const bytes = randomBytes(size);
  let out = '';
  for (let i = 0; i < bytes.length; i += 1) {
    out += ALPHABET[bytes[i] % ALPHABET.length];
  }
  return out;
}

export function newId(prefix) {
  if (typeof prefix !== 'string' || prefix.length === 0) {
    throw new TypeError('an id prefix is required');
  }
  return `${prefix}_${randomToken(10)}`;
}

export function isId(value, prefix) {
  if (typeof value !== 'string' || value.length > 80) return false;
  const parts = value.split('_');
  if (parts.length !== 2) return false;
  const [head, tail] = parts;
  if (prefix !== undefined && head !== prefix) return false;
  return /^[0-9a-hjkmnp-tv-z]{10}$/.test(tail);
}

export function slugify(text) {
  return String(text ?? '')
    .normalize('NFKD')
    .replace(/[\u0300-\u036f]/g, '')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 48);
}

export function fingerprint(parts) {
  const joined = parts.map((part) => String(part ?? '')).join('\u0000');
  return createHash('sha256').update(joined).digest('hex').slice(0, 32);
}

export function compareIds(left, right) {
  if (left === right) return 0;
  return left < right ? -1 : 1;
}

export function sortIds(ids) {
  return [...ids].sort(compareIds);
}
