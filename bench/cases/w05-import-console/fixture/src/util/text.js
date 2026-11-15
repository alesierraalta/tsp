export function normalizeSpace(value) {
  return String(value ?? '').replace(/\s+/g, ' ').trim();
}

export function trimTo(value, max) {
  const text = String(value ?? '');
  if (text.length <= max) return text;
  return `${text.slice(0, Math.max(0, max - 1))}\u2026`;
}

export function titleCase(value) {
  return normalizeSpace(value)
    .split(' ')
    .filter(Boolean)
    .map((word) => word[0].toUpperCase() + word.slice(1).toLowerCase())
    .join(' ');
}

export function canonicalKey(value) {
  return normalizeSpace(value)
    .normalize('NFKC')
    .toLowerCase()
    .replace(/[\u2010-\u2015]/g, '-');
}

export function canonicalHeader(value) {
  return canonicalKey(value).replace(/[^a-z0-9]+/g, '_').replace(/^_+|_+$/g, '');
}

export function pluralize(count, one, many) {
  return `${count} ${count === 1 ? one : many}`;
}

export function splitOnce(value, separator) {
  const text = String(value ?? '');
  const index = text.indexOf(separator);
  if (index < 0) return [text, ''];
  return [text.slice(0, index), text.slice(index + separator.length)];
}

export function pad(value, width, filler = ' ') {
  const text = String(value ?? '');
  if (text.length >= width) return text;
  return text + filler.repeat(width - text.length);
}

export function indentLines(text, spaces) {
  const padText = ' '.repeat(spaces);
  return String(text)
    .split('\n')
    .map((line) => `${padText}${line}`)
    .join('\n');
}

export function chunk(items, size) {
  if (!Array.isArray(items)) throw new TypeError('chunk expects a list');
  const step = Math.max(1, Math.floor(size));
  const out = [];
  for (let index = 0; index < items.length; index += step) {
    out.push(items.slice(index, index + step));
  }
  return out;
}

export function slug(value) {
  return canonicalKey(value).replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '');
}

export function initials(value) {
  return normalizeSpace(value)
    .split(' ')
    .filter(Boolean)
    .slice(0, 2)
    .map((word) => word[0].toUpperCase())
    .join('');
}

export function compareText(left, right) {
  const a = canonicalKey(left);
  const b = canonicalKey(right);
  if (a === b) return 0;
  return a < b ? -1 : 1;
}
