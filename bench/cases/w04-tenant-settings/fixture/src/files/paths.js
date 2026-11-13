import { isAbsolute, join, relative, resolve, sep } from 'node:path';
import { unsafePath } from '../http/errors.js';

export const NAME_PATTERN = /^[A-Za-z0-9][A-Za-z0-9._-]{0,120}$/;

export function assertPlainName(name) {
  const text = String(name ?? '');
  if (text.length === 0 || text.length > 121) throw unsafePath(name);
  if (text.includes('/') || text.includes('\\') || text.includes('\u0000')) throw unsafePath(name);
  if (text === '.' || text === '..') throw unsafePath(name);
  return text;
}

export function resolveUnder(root, name) {
  const base = resolve(String(root));
  const text = String(name ?? '');
  if (text.length === 0 || text.includes('\u0000')) throw unsafePath(name);
  if (isAbsolute(text)) throw unsafePath(name);
  const candidate = resolve(join(base, text));
  const inside = candidate === base || candidate.startsWith(base + sep);
  if (!inside) throw unsafePath(name);
  return candidate;
}

export function relativeUnder(root, file) {
  const base = resolve(String(root));
  const target = resolve(String(file));
  const rel = relative(base, target);
  if (rel.startsWith('..') || isAbsolute(rel)) throw unsafePath(file);
  return rel;
}

export function extensionOf(name) {
  const text = assertPlainName(name);
  const index = text.lastIndexOf('.');
  if (index <= 0) return '';
  return text.slice(index + 1).toLowerCase();
}

export function baseName(name) {
  const text = assertPlainName(name);
  return text.replace(/\.[^.]+$/, '');
}

export function joinUnder(root, ...parts) {
  return parts.reduce((acc, part) => resolveUnder(acc, part), resolve(String(root)));
}

export const MIME = {
  json: 'application/json; charset=utf-8',
  csv: 'text/csv; charset=utf-8',
  txt: 'text/plain; charset=utf-8',
  html: 'text/html; charset=utf-8',
  md: 'text/markdown; charset=utf-8',
  png: 'image/png',
};

export function mimeOf(name) {
  return MIME[extensionOf(name)] ?? 'application/octet-stream';
}

export function isArchiveName(name) {
  const ext = extensionOf(name);
  return ext === 'json' || ext === 'csv';
}
