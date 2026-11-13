import { tooLarge } from '../util/errors.js';

export function queryOf(url) {
  const out = {};
  for (const [key, value] of url.searchParams.entries()) {
    if (Object.prototype.hasOwnProperty.call(out, key)) {
      out[key] = Array.isArray(out[key]) ? [...out[key], value] : [out[key], value];
    } else {
      out[key] = value;
    }
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
  if (['1', 'true', 'yes'].includes(text)) return true;
  if (['0', 'false', 'no'].includes(text)) return false;
  return fallback;
}

export function clientOf(req) {
  const forwarded = req.headers['x-forwarded-for'];
  if (typeof forwarded === 'string' && forwarded.trim() !== '') return forwarded.split(',')[0].trim();
  return req.socket?.remoteAddress ?? '127.0.0.1';
}

export async function readBody(req, { limit = 1024 * 1024 } = {}) {
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
    throw new TypeError(`the request body is not valid JSON: ${error.message}`);
  }
}

export async function readNdjson(req, options = {}) {
  const body = await readBody(req, options);
  const lines = body.toString('utf8').split('\n').filter((line) => line.trim() !== '');
  return lines.map((line, index) => {
    try {
      return JSON.parse(line);
    } catch (error) {
      throw new TypeError(`line ${index + 1} is not valid JSON: ${error.message}`);
    }
  });
}

export function rangeOf(query, total) {
  const offset = Math.max(0, queryInt(query, 'offset', 0));
  const limit = Math.max(1, Math.min(500, queryInt(query, 'limit', 50)));
  return { offset: Math.min(offset, total), limit, end: Math.min(offset + limit, total) };
}

export function rangeParam(query, name, fallback) {
  const raw = String(query?.[name] ?? '');
  const match = /^(\d{1,9})-(\d{1,9})$/.exec(raw);
  if (!match) return fallback;
  const from = Number(match[1]);
  const to = Number(match[2]);
  if (!Number.isFinite(from) || !Number.isFinite(to) || to < from) return fallback;
  return { from, to };
}

export function pageParams(query, options = {}) {
  const maxLimit = options.maxLimit ?? 500;
  const limit = Math.max(1, Math.min(maxLimit, queryInt(query, 'limit', options.limit ?? 50)));
  const offset = Math.max(0, queryInt(query, 'offset', 0));
  return { limit, offset };
}

export function pageSlice(items, page) {
  const slice = (items ?? []).slice(page.offset, page.offset + page.limit);
  return { items: slice, total: (items ?? []).length, has_more: page.offset + slice.length < (items ?? []).length };
}

export function bodyLimitOf(options = {}) {
  const limit = Number(options.limit ?? 1024 * 1024);
  return Number.isFinite(limit) && limit > 0 ? limit : 1024 * 1024;
}
