import { randomBytes, createHash } from 'node:crypto';

const ALPHABET = '0123456789abcdefghijklmnopqrstuvwxyz';

export function randomHex(size = 8) {
  return randomBytes(Math.ceil(size / 2)).toString('hex').slice(0, size);
}

export function newId(prefix = 'id') {
  return `${prefix}_${randomHex(16)}`;
}

export function keyId(prefix = 'key') {
  return `${prefix}_${randomHex(24)}`;
}

export function deliveryId(prefix = 'dlv') {
  return `${prefix}_${randomHex(20)}`;
}

export function isId(value, prefix) {
  return new RegExp(`^${prefix}_[0-9a-f]{6,32}$`).test(String(value ?? ''));
}

export function idPrefix(value) {
  const index = String(value ?? '').indexOf('_');
  return index <= 0 ? '' : String(value).slice(0, index);
}

export function shortHash(value, size = 8) {
  return createHash('sha256').update(String(value ?? ''), 'utf8').digest('hex').slice(0, size);
}

export function stableId(prefix, value) {
  return `${prefix}_${shortHash(value, 10)}`;
}

export function sequenceId(prefix, name, sequence) {
  const base = String(name ?? '')
    .split('')
    .reduce((acc, char) => (acc * 33 + char.charCodeAt(0)) % 0xffffffff, 7)
    .toString(16)
    .padStart(6, '0')
    .slice(0, 6);
  return `${prefix}_${base}${String(sequence).padStart(5, '0')}`;
}

export function sortableId(instant, suffix = '') {
  const time = new Date(instant ?? Date.now()).toISOString().replace(/[-:.TZ]/g, '').slice(0, 14);
  return `id_${time}${suffix}`;
}

export function randomToken(size = 24) {
  let out = '';
  const bytes = randomBytes(size);
  for (const byte of bytes) out += ALPHABET[byte % ALPHABET.length];
  return out;
}
