import { statusOf } from '../util/errors.js';

const BASE_HEADERS = {
  'cache-control': 'no-store',
  'x-content-type-options': 'nosniff',
};

export function send(res, status, body, headers = {}) {
  if (res.writableEnded) return;
  const payload = Buffer.isBuffer(body) ? body : Buffer.from(String(body ?? ''), 'utf8');
  res.writeHead(status, { ...BASE_HEADERS, 'content-length': payload.length, ...headers });
  res.end(payload);
}

export function json(res, status, body, headers = {}) {
  const payload = JSON.stringify(body ?? null);
  send(res, status, payload, { 'content-type': 'application/json; charset=utf-8', ...headers });
}

export function text(res, status, body, headers = {}) {
  send(res, status, body, { 'content-type': 'text/plain; charset=utf-8', ...headers });
}

export function html(res, status, body, headers = {}) {
  send(res, status, body, { 'content-type': 'text/html; charset=utf-8', ...headers });
}

export function csv(res, status, body, name, headers = {}) {
  send(res, status, body, {
    'content-type': 'text/csv; charset=utf-8',
    'content-disposition': `attachment; filename="${name}"`,
    ...headers,
  });
}

export function redirect(res, location) {
  send(res, 302, '', { location });
}

export function noContent(res) {
  send(res, 204, '');
}

export function sendError(res, error, options = {}) {
  const status = statusOf(error);
  const body =
    error && typeof error.toJSON === 'function'
      ? error.toJSON()
      : { error: 'internal_error', message: options.expose === true ? String(error?.message ?? error) : 'internal error' };
  if (options.logger) options.logger.error('request failed', { status, message: String(error?.message ?? error) });
  json(res, status, body);
}

export function sseChunk(event, data) {
  return `event: ${event}\ndata: ${JSON.stringify(data)}\n\n`;
}

export function setHeader(res, name, value) {
  res.setHeader(name, value);
}

export function notModified(res, etag) {
  send(res, 304, '', { etag });
}

export function etagOf(body) {
  return `"${Buffer.byteLength(String(body ?? ''))}-${String(body ?? '').length}"`;
}

export function sendWithEtag(req, res, body, headers = {}) {
  const etag = etagOf(body);
  if (req.headers['if-none-match'] === etag) {
    notModified(res, etag);
    return;
  }
  send(res, 200, body, { etag, ...headers });
}

export function jsonLines(res, rows, headers = {}) {
  const body = rows.map((row) => JSON.stringify(row)).join('\n');
  send(res, 200, body === '' ? '' : `${body}\n`, {
    'content-type': 'application/x-ndjson; charset=utf-8',
    ...headers,
  });
}

export function emptyJsonArray(res, meta = {}) {
  json(res, 200, { items: [], ...meta });
}

export function accepted(res, body) {
  json(res, 202, body);
}

export function partialContent(res, body, range, total) {
  send(res, 206, body, {
    'content-range': `bytes ${range.start}-${range.end}/${total}`,
    'accept-ranges': 'bytes',
  });
}

export function parseRange(header, total) {
  const match = /^bytes=(\d*)-(\d*)$/.exec(String(header ?? '').trim());
  if (!match) return undefined;
  const start = match[1] === '' ? 0 : Number(match[1]);
  const end = match[2] === '' ? total - 1 : Math.min(Number(match[2]), total - 1);
  if (!Number.isFinite(start) || !Number.isFinite(end) || start > end || start >= total) return undefined;
  return { start, end };
}

export function problem(res, status, code, message, details) {
  json(res, status, { type: `https://errors.example.invalid/${code}`, title: message, status, error: code, details });
}
