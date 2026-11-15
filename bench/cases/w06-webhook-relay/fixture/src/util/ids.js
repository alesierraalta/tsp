import { createHash, randomBytes } from 'node:crypto';

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

export function fingerprint(parts) {
  const payload = parts.map((part) => String(part ?? '')).join('\u0000');
  return createHash('sha256').update(payload).digest('hex').slice(0, 24);
}

export function idempotencyKey(job) {
  return fingerprint([job.event, job.target, job.entity_id, job.topic]);
}

export function shortHash(value, length = 8) {
  return createHash('sha256').update(String(value ?? '')).digest('hex').slice(0, length);
}
