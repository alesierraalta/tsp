import { randomUUID } from 'node:crypto';

export function newId(prefix = 'id') {
  return `${prefix}_${randomUUID().replace(/-/g, '').slice(0, 16)}`;
}

export function eventId(prefix = 'evt') {
  return `${prefix}_${randomUUID().replace(/-/g, '').slice(0, 20)}`;
}

export function isId(value, prefix) {
  return new RegExp(`^${prefix}_[0-9a-f]{8,32}$`).test(String(value ?? ''));
}

export function idPrefix(value) {
  const index = String(value ?? '').indexOf('_');
  return index <= 0 ? '' : String(value).slice(0, index);
}

export function hexOf(value, size = 8) {
  return String(value ?? '')
    .split('')
    .reduce((acc, char) => (acc * 31 + char.charCodeAt(0)) % 0xffffffff, 7)
    .toString(16)
    .padStart(size, '0')
    .slice(0, size);
}

export function batchId(tenant, sequence) {
  return `bat_${hexOf(tenant, 6)}${String(sequence).padStart(6, '0')}`;
}

export function keyId(prefix = 'key') {
  return `${prefix}_${randomUUID().replace(/-/g, '').slice(0, 24)}`;
}
