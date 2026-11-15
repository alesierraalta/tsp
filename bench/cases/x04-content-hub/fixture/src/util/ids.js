import { randomUUID } from 'node:crypto';

const ALPHABET = '0123456789abcdefghijklmnopqrstuvwxyz';

export function newId(prefix = 'id') {
  const body = randomUUID().replace(/-/g, '').slice(0, 16);
  return `${prefix}_${body}`;
}

export function shortId(prefix = 'id', size = 8) {
  return `${prefix}_${randomUUID().replace(/-/g, '').slice(0, size)}`;
}

export function encodeBase36(value) {
  const number = Math.max(0, Math.trunc(Number(value) || 0));
  if (number === 0) return '0';
  let rest = number;
  let out = '';
  while (rest > 0) {
    out = ALPHABET[rest % 36] + out;
    rest = Math.floor(rest / 36);
  }
  return out;
}

export function decodeBase36(text) {
  const clean = String(text ?? '').toLowerCase();
  let total = 0;
  for (const char of clean) {
    const digit = ALPHABET.indexOf(char);
    if (digit < 0) return Number.NaN;
    total = total * 36 + digit;
  }
  return total;
}

export function monotonic(prefix, sequence, size = 6) {
  return `${prefix}_${String(sequence).padStart(size, '0')}`;
}

export function isId(value, prefix) {
  const pattern = new RegExp(`^${prefix}_[0-9a-z]{4,32}$`);
  return pattern.test(String(value ?? ''));
}

export function idPrefix(value) {
  const text = String(value ?? '');
  const index = text.indexOf('_');
  return index <= 0 ? '' : text.slice(0, index);
}
