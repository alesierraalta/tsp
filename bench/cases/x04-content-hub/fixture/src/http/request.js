import { tooLarge } from '../util/errors.js';

export const MAX_BODY_BYTES = 512 * 1024;

export function queryOf(url) {
  const out = {};
  for (const [key, value] of url.searchParams.entries()) {
    if (Object.prototype.hasOwnProperty.call(out, key)) {
      const existing = out[key];
      out[key] = Array.isArray(existing) ? [...existing, value] : [existing, value];
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

export function queryBool(query, name, fallback = false) {
  const raw = query?.[name];
  const text = String(Array.isArray(raw) ? raw[raw.length - 1] : (raw ?? '')).toLowerCase();
  if (text === '1' || text === 'true' || text === 'yes') return true;
  if (text === '0' || text === 'false' || text === 'no') return false;
  return fallback;
}

export function queryList(query, name) {
  const raw = query?.[name];
  if (raw === undefined) return [];
  const values = Array.isArray(raw) ? raw : [raw];
  return values
    .flatMap((value) => String(value).split(','))
    .map((value) => value.trim())
    .filter(Boolean);
}

export function clientOf(req) {
  const forwarded = req.headers['x-forwarded-for'];
  if (typeof forwarded === 'string' && forwarded.trim() !== '') {
    return forwarded.split(',')[0].trim();
  }
  return req.socket?.remoteAddress ?? '127.0.0.1';
}

export async function readBody(req, { limit = MAX_BODY_BYTES } = {}) {
  const declared = Number(req.headers['content-length'] ?? 0);
  if (Number.isFinite(declared) && declared > limit) {
    throw tooLarge(`a request body holds at most ${limit} bytes`);
  }
  const chunks = [];
  let total = 0;
  for await (const chunk of req) {
    total += chunk.length;
    if (total > limit) throw tooLarge(`a request body holds at most ${limit} bytes`);
    chunks.push(chunk);
  }
  return Buffer.concat(chunks);
}

export async function readJson(req, options = {}) {
  const body = await readBody(req, options);
  if (body.length === 0) return {};
  const text = body.toString('utf8');
  try {
    return JSON.parse(text);
  } catch (error) {
    throw new TypeError(`request body is not valid JSON: ${error.message}`);
  }
}

export function pathParts(path) {
  return String(path ?? '/')
    .split('/')
    .filter((part) => part.length > 0);
}

export function rangeOf(query, total) {
  const offset = Math.max(0, queryInt(query, 'offset', 0));
  const limit = Math.max(1, Math.min(200, queryInt(query, 'limit', 20)));
  return { offset: Math.min(offset, total), limit, end: Math.min(offset + limit, total) };
}
