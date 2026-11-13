import { statusOf } from '../util/errors.js';

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

export function ndjson(res, status, rows, headers = {}) {
  const body = rows.map((row) => JSON.stringify(row)).join('\n');
  send(res, status, body === '' ? '' : `${body}\n`, { 'content-type': 'application/x-ndjson; charset=utf-8', ...headers });
}

export function csvDownload(res, body, name) {
  send(res, 200, body, {
    'content-type': 'text/csv; charset=utf-8',
    'content-disposition': `attachment; filename="${name}"`,
  });
}

export function noContent(res) {
  send(res, 204, '');
}

export function accepted(res, body) {
  json(res, 202, body);
}

export function sendError(res, error, options = {}) {
  const status = statusOf(error);
  const body =
    typeof error?.toJSON === 'function'
      ? error.toJSON()
      : { error: 'internal_error', message: options.expose ? String(error?.message ?? error) : 'internal error' };
  if (options.logger) options.logger.error('request failed', { status, message: String(error?.message ?? error) });
  json(res, status, body);
}

export function problem(res, status, code, message, details) {
  json(res, status, { type: `https://errors.example.invalid/${code}`, title: message, status, error: code, details });
}

export function parseRange(header, total) {
  const match = /^bytes=(\d*)-(\d*)$/.exec(String(header ?? '').trim());
  if (!match) return undefined;
  const start = match[1] === '' ? 0 : Number(match[1]);
  const end = match[2] === '' ? total - 1 : Math.min(Number(match[2]), total - 1);
  return Number.isFinite(start) && start <= end && start < total ? { start, end } : undefined;
}

export function jsonLinesSummary(rows) {
  return { rows: rows.length, bytes: rows.reduce((total, row) => total + Buffer.byteLength(JSON.stringify(row)), 0) };
}
