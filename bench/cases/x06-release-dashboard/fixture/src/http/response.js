import { statusOf, codeOf } from '../util/errors.js';

const BASE = { 'x-content-type-options': 'nosniff', 'cache-control': 'no-store' };

export function send(res, status, body, headers = {}) {
  if (res.writableEnded) return;
  const payload = Buffer.isBuffer(body) ? body : Buffer.from(String(body ?? ''), 'utf8');
  res.writeHead(status, { ...BASE, 'content-length': payload.length, ...headers });
  res.end(payload);
}

export function json(res, status, body, headers = {}) {
  send(res, status, JSON.stringify(body ?? null), { 'content-type': 'application/json; charset=utf-8', ...headers });
}

export function text(res, status, body, headers = {}) {
  send(res, status, body, { 'content-type': 'text/plain; charset=utf-8', ...headers });
}

export function html(res, status, body, headers = {}) {
  send(res, status, body, { 'content-type': 'text/html; charset=utf-8', ...headers });
}

export function noContent(res) {
  send(res, 204, '');
}

export function created(res, body) {
  json(res, 201, body);
}

export function accepted(res, body) {
  json(res, 202, body);
}

export function redirect(res, location) {
  send(res, 302, '', { location });
}

export function download(res, body, name, type = 'text/csv; charset=utf-8') {
  send(res, 200, body, { 'content-type': type, 'content-disposition': `attachment; filename="${name}"` });
}

export function sendError(res, error, options = {}) {
  const status = statusOf(error);
  const body =
    typeof error?.toJSON === 'function'
      ? error.toJSON()
      : { error: codeOf(error), message: options.expose ? String(error?.message ?? error) : 'internal error' };
  if (options.logger) options.logger.error('request failed', { status, message: String(error?.message ?? error) });
  json(res, status, body);
}

export function problem(res, status, code, message, details) {
  json(res, status, { type: `https://errors.example.invalid/${code}`, title: message, status, error: code, details });
}

export function etagOf(value) {
  const text = typeof value === 'string' ? value : JSON.stringify(value ?? null);
  let hash = 7;
  for (const char of text) hash = (hash * 31 + char.charCodeAt(0)) % 0xffffffff;
  return `W/"${hash.toString(16)}"`;
}
