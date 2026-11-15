const HTML_ESCAPES = {
  '&': '&amp;',
  '<': '&lt;',
  '>': '&gt;',
  '"': '&quot;',
  "'": '&#39;',
};

export function escapeHtml(value) {
  return String(value ?? '').replace(/[&<>"']/g, (char) => HTML_ESCAPES[char]);
}

export function escapeAttr(value) {
  return escapeHtml(value).replace(/`/g, '&#96;');
}

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

export function maskSecret(value, keep = 4) {
  const text = String(value ?? '');
  if (text.length <= keep) return '*'.repeat(text.length);
  return `${'*'.repeat(text.length - keep)}${text.slice(-keep)}`;
}

export function parseList(value) {
  if (Array.isArray(value)) return value.flatMap((entry) => parseList(entry));
  if (typeof value !== 'string') return [];
  return value
    .split(',')
    .map((entry) => entry.trim())
    .filter(Boolean);
}

export function slugWord(value) {
  return normalizeSpace(value).toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '');
}

export function plural(count, one, many) {
  return count === 1 ? one : many;
}

export function highlight(value, needle) {
  const text = escapeHtml(value);
  const term = escapeHtml(needle);
  if (!term) return text;
  return text.split(term).join(`<mark>${term}</mark>`);
}

export function joinLines(lines) {
  return lines.filter((line) => line !== undefined && line !== null).join('\n');
}

export function indent(text, spaces) {
  const pad = ' '.repeat(spaces);
  return String(text)
    .split('\n')
    .map((line) => `${pad}${line}`)
    .join('\n');
}
