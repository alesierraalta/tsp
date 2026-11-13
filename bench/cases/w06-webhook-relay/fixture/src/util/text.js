export function normalizeSpace(value) {
  return String(value ?? '').replace(/\s+/g, ' ').trim();
}

export function canonical(value) {
  return normalizeSpace(value).normalize('NFKC').toLowerCase();
}

export function trimTo(value, max) {
  const text = String(value ?? '');
  return text.length <= max ? text : `${text.slice(0, Math.max(0, max - 1))}\u2026`;
}

export function slug(value) {
  return canonical(value).replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '');
}

export function capitalize(value) {
  const text = normalizeSpace(value);
  if (text.length === 0) return '';
  return text[0].toUpperCase() + text.slice(1);
}

export function plural(count, one, many) {
  return `${count} ${count === 1 ? one : many}`;
}

export function parseList(value) {
  if (Array.isArray(value)) return value.flatMap((entry) => parseList(entry));
  if (typeof value !== 'string') return [];
  return value
    .split(',')
    .map((entry) => entry.trim())
    .filter(Boolean);
}

export function maskSecret(value, keep = 4) {
  const text = String(value ?? '');
  if (text.length <= keep) return '*'.repeat(text.length);
  return `${'*'.repeat(text.length - keep)}${text.slice(-keep)}`;
}

export function redactHeaders(headers) {
  const out = {};
  for (const [key, value] of Object.entries(headers ?? {})) {
    out[key] = /authorization|signature|secret|token/i.test(key) ? '[redacted]' : value;
  }
  return out;
}

export function firstLine(value) {
  return normalizeSpace(value).split('\n')[0];
}

export function bytesToText(bytes) {
  return `${(Number(bytes) / 1024).toFixed(1)} KiB`;
}

export function compareText(left, right) {
  const a = canonical(left);
  const b = canonical(right);
  if (a === b) return 0;
  return a < b ? -1 : 1;
}
