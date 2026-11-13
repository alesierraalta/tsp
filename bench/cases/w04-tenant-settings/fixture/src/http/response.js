import { asHttpError } from './errors.js';
import { escapeHtml } from '../util/text.js';

export function applyHeaders(res, headers) {
  for (const [key, value] of Object.entries(headers ?? {})) {
    if (value === undefined) continue;
    res.setHeader(key, String(value));
  }
}

export function json(res, status, payload, headers = {}) {
  const body = JSON.stringify(payload ?? null);
  res.statusCode = status;
  res.setHeader('content-type', 'application/json; charset=utf-8');
  res.setHeader('content-length', Buffer.byteLength(body));
  applyHeaders(res, headers);
  res.end(body);
  return body;
}

export function text(res, status, body, headers = {}) {
  const payload = String(body ?? '');
  res.statusCode = status;
  res.setHeader('content-type', 'text/plain; charset=utf-8');
  res.setHeader('content-length', Buffer.byteLength(payload));
  applyHeaders(res, headers);
  res.end(payload);
  return payload;
}

export function html(res, status, markup, headers = {}) {
  const body = String(markup ?? '');
  res.statusCode = status;
  res.setHeader('content-type', 'text/html; charset=utf-8');
  res.setHeader('content-length', Buffer.byteLength(body));
  applyHeaders(res, headers);
  res.end(body);
  return body;
}

export function empty(res, status, headers = {}) {
  res.statusCode = status;
  applyHeaders(res, headers);
  res.end();
}

export function redirect(res, location, status = 302) {
  empty(res, status, { location });
}

export function errorPage(error) {
  return [
    '<!doctype html>',
    '<html lang="en">',
    '<head><meta charset="utf-8"><title>Request failed</title></head>',
    '<body>',
    '<h1>Request failed</h1>',
    `<p class="code">${escapeHtml(error.code ?? 'error')}</p>`,
    `<p class="message">${escapeHtml(error.message ?? 'unknown error')}</p>`,
    '</body>',
    '</html>',
  ].join('\n');
}

export function sendError(res, error, options = {}) {
  const logger = options.logger;
  const known = asHttpError(error);
  if (known.status >= 500 && logger) {
    logger.error('request failed', {
      code: known.code,
      status: known.status,
      message: known.message,
    });
  }
  if (options.asHtml) {
    html(res, known.status, errorPage(known));
    return;
  }
  json(res, known.status, known.toJSON());
}

export function noContent(res) {
  empty(res, 204);
}

export function accepted(res, payload) {
  json(res, 202, payload);
}

export function created(res, payload, location) {
  const headers = {};
  if (location) headers.location = location;
  json(res, 201, payload, headers);
}
