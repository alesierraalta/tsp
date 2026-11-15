export function singleLine(text) {
  return String(text ?? '').replace(/\s+/g, ' ').trim();
}

export function truncate(text, limit = 120) {
  const value = String(text ?? '');
  return value.length <= limit ? value : `${value.slice(0, Math.max(0, limit - 1))}…`;
}

export function slugify(text) {
  return String(text ?? '')
    .normalize('NFKD')
    .replace(/[\u0300-\u036f]/g, '')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 80);
}

export function splitCsv(text, separator = ',') {
  return String(text ?? '')
    .split(separator)
    .map((part) => part.trim())
    .filter(Boolean);
}

export function isBlank(value) {
  return value === null || value === undefined || String(value).trim() === '';
}

export function initials(name) {
  return singleLine(name)
    .split(' ')
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0].toUpperCase())
    .join('');
}

export function camelCase(text) {
  return String(text ?? '')
    .split(/[^A-Za-z0-9]+/)
    .filter(Boolean)
    .map((part, index) => (index === 0 ? part.toLowerCase() : part[0].toUpperCase() + part.slice(1).toLowerCase()))
    .join('');
}

export function snakeCase(text) {
  return String(text ?? '')
    .replace(/([a-z0-9])([A-Z])/g, '$1_$2')
    .replace(/[\s.-]+/g, '_')
    .toLowerCase();
}

export function titleCase(text) {
  return singleLine(text)
    .split(' ')
    .map((word) => (word === '' ? word : word[0].toUpperCase() + word.slice(1)))
    .join(' ');
}

export function wordCount(text) {
  return singleLine(text).split(' ').filter(Boolean).length;
}

export function excerpt(text, words = 20) {
  const parts = singleLine(text).split(' ').filter(Boolean);
  return parts.length <= words ? parts.join(' ') : `${parts.slice(0, words).join(' ')}…`;
}

export function maskSecret(secret) {
  const value = String(secret ?? '');
  if (value.length <= 8) return '*'.repeat(value.length);
  return `${value.slice(0, 4)}${'*'.repeat(Math.max(4, value.length - 8))}${value.slice(-4)}`;
}

export function normalizeKey(text) {
  return singleLine(text).toLowerCase();
}

export function containsAny(haystack, needles) {
  const value = String(haystack ?? '').toLowerCase();
  return needles.some((needle) => value.includes(String(needle).toLowerCase()));
}

export function groupWords(text, size = 3) {
  const words = singleLine(text).split(' ').filter(Boolean);
  const out = [];
  for (let index = 0; index + size <= words.length; index += 1) out.push(words.slice(index, index + size).join(' '));
  return out;
}
