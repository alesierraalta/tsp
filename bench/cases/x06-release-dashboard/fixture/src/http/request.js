import { tooLarge, badRequest } from '../util/errors.js';

export function queryOf(url) {
  const out = {};
  for (const [name, value] of url.searchParams.entries()) {
    if (Object.prototype.hasOwnProperty.call(out, name)) {
      out[name] = Array.isArray(out[name]) ? [...out[name], value] : [out[name], value];
      continue;
    }
    out[name] = value;
  }
  return out;
}

export function queryInt(query, name, fallback) {
  const raw = query?.[name];
  const text = Array.isArray(raw) ? raw[raw.length - 1] : raw;
  const value = Number.parseInt(String(text ?? ''), 10);
  return Number.isFinite(value) ? value : fallback;
}

export function queryList(query, name) {
  const raw = query?.[name];
  if (raw === undefined) return [];
  return (Array.isArray(raw) ? raw : [raw])
    .flatMap((value) => String(value).split(','))
    .map((value) => value.trim())
    .filter(Boolean);
}

export function queryBool(query, name, fallback = false) {
  const raw = query?.[name];
  const text = String(Array.isArray(raw) ? raw[raw.length - 1] : (raw ?? '')).toLowerCase();
  if (['1', 'true', 'yes', 'on'].includes(text)) return true;
  if (['0', 'false', 'no', 'off'].includes(text)) return false;
  return fallback;
}

export function pageParams(query, options = {}) {
  const maxLimit = options.maxLimit ?? 200;
  const limit = Math.max(1, Math.min(maxLimit, queryInt(query, 'limit', options.limit ?? 50)));
  const offset = Math.max(0, queryInt(query, 'offset', 0));
  return { limit, offset };
}

export function rangeOf(query, total) {
  const offset = Math.max(0, queryInt(query, 'offset', 0));
  const limit = Math.max(1, Math.min(200, queryInt(query, 'limit', 50)));
  return { offset: Math.min(offset, total), limit, end: Math.min(offset + limit, total) };
}

export function clientOf(req) {
  const forwarded = req.headers['x-forwarded-for'];
  if (typeof forwarded === 'string' && forwarded.trim() !== '') return forwarded.split(',')[0].trim();
  return req.socket?.remoteAddress ?? '127.0.0.1';
}

export async function readBody(req, options = {}) {
  const limit = Number(options.limit ?? 1024 * 1024);
  const declared = Number(req.headers['content-length'] ?? 0);
  if (Number.isFinite(declared) && declared > limit) throw tooLarge(`a body holds at most ${limit} bytes`);
  const chunks = [];
  let total = 0;
  for await (const chunk of req) {
    total += chunk.length;
    if (total > limit) throw tooLarge(`a body holds at most ${limit} bytes`);
    chunks.push(chunk);
  }
  return Buffer.concat(chunks);
}

export async function readJson(req, options = {}) {
  const body = await readBody(req, options);
  if (body.length === 0) return {};
  try {
    return JSON.parse(body.toString('utf8'));
  } catch (error) {
    throw badRequest('invalid_json', `the request body is not valid JSON: ${error.message}`);
  }
}

export async function readText(req, options = {}) {
  const body = await readBody(req, options);
  return body.toString('utf8');
}

export function headerValue(req, name, fallback = '') {
  const value = req.headers[String(name).toLowerCase()];
  return value === undefined ? fallback : String(value);
}
