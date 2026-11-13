export function singleLine(text) {
  return String(text ?? '')
    .replace(/\s+/g, ' ')
    .trim();
}

export function truncate(text, limit = 120) {
  const value = String(text ?? '');
  if (value.length <= limit) return value;
  return `${value.slice(0, Math.max(0, limit - 1))}…`;
}

export function slugify(text) {
  return String(text ?? '')
    .normalize('NFKD')
    .replace(/[\u0300-\u036f]/g, '')
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 64);
}

export function titleCase(text) {
  return singleLine(text)
    .split(' ')
    .map((word) => (word === '' ? word : word[0].toUpperCase() + word.slice(1)))
    .join(' ');
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

export function initials(name) {
  return singleLine(name)
    .split(' ')
    .filter(Boolean)
    .slice(0, 2)
    .map((part) => part[0].toUpperCase())
    .join('');
}

export function isBlank(value) {
  return value === null || value === undefined || String(value).trim() === '';
}

export function maskSecret(secret) {
  const value = String(secret ?? '');
  if (value.length <= 8) return '*'.repeat(value.length);
  return `${value.slice(0, 4)}${'*'.repeat(Math.max(4, value.length - 8))}${value.slice(-4)}`;
}

export function containsAny(haystack, needles) {
  const value = String(haystack ?? '').toLowerCase();
  return (needles ?? []).some((needle) => value.includes(String(needle).toLowerCase()));
}

export function splitList(text, separator = ',') {
  return String(text ?? '')
    .split(separator)
    .map((part) => part.trim())
    .filter(Boolean);
}

export function wordCount(text) {
  return singleLine(text).split(' ').filter(Boolean).length;
}

export function excerpt(text, words = 24) {
  const parts = singleLine(text).split(' ').filter(Boolean);
  return parts.length <= words ? parts.join(' ') : `${parts.slice(0, words).join(' ')}…`;
}

export function bulletList(lines, marker = '-') {
  return (lines ?? []).map((line) => `${marker} ${singleLine(line)}`).join('\n');
}

export function pad(text, width) {
  const value = String(text ?? '');
  return value.length >= width ? value : value + ' '.repeat(width - value.length);
}

export function repeatTo(text, width) {
  const value = String(text ?? '');
  if (value === '') return '';
  return value.repeat(Math.ceil(width / value.length)).slice(0, width);
}

export function pluralize(count, singular, plural) {
  return `${count} ${count === 1 ? singular : (plural ?? `${singular}s`)}`;
}

export function alignTable(rows, options = {}) {
  const list = (rows ?? []).map((row) => (Array.isArray(row) ? row.map((cell) => String(cell ?? '')) : []));
  if (list.length === 0) return '';
  const widths = [];
  for (const row of list) {
    row.forEach((cell, index) => {
      widths[index] = Math.max(widths[index] ?? 0, cell.length);
    });
  }
  const separator = (options.separator ?? '  ').slice(0, 2);
  return list.map((row) => row.map((cell, index) => pad(cell, widths[index])).join(separator).trimEnd()).join('\n');
}

export function firstSentence(text) {
  const value = singleLine(text);
  const end = value.search(/[.!?](\s|$)/);
  return end < 0 ? value : value.slice(0, end + 1);
}

export function joinWithAnd(parts, conjunction = 'and') {
  const list = (parts ?? []).filter(Boolean).map((part) => String(part));
  if (list.length === 0) return '';
  if (list.length === 1) return list[0];
  return `${list.slice(0, -1).join(', ')} ${conjunction} ${list[list.length - 1]}`;
}

export function quote(text) {
  return `"${singleLine(text)}"`;
}
