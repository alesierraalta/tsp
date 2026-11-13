import { HttpError, badRequest, tooLarge, unsupportedMedia } from './errors.js';
import { isForm, isJson, parseHeaders } from './headers.js';

export const DEFAULT_BODY_LIMIT = 64 * 1024;

const FORBIDDEN_KEYS = new Set(['__proto__', 'constructor', 'prototype']);

export function readBody(req, options = {}) {
  const limit = options.limit ?? DEFAULT_BODY_LIMIT;
  const timeoutMs = options.timeoutMs ?? 5000;
  const headers = parseHeaders(req.headers);
  const declared = Number(headers['content-length'] ?? '');
  if (Number.isFinite(declared) && declared > limit) {
    req.resume?.();
    return Promise.reject(tooLarge(limit));
  }

  return new Promise((resolve, reject) => {
    const chunks = [];
    let size = 0;
    let settled = false;

    const finish = (fn, value) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      req.off?.('data', onData);
      req.off?.('end', onEnd);
      req.off?.('error', onError);
      fn(value);
    };

    const timer = setTimeout(() => {
      finish(reject, new HttpError(408, 'body_timeout', 'request body timed out'));
      req.destroy?.();
    }, timeoutMs);

    function onData(chunk) {
      size += chunk.length;
      if (size > limit) {
        finish(reject, tooLarge(limit));
        req.destroy?.();
        return;
      }
      chunks.push(chunk);
    }

    function onEnd() {
      finish(resolve, Buffer.concat(chunks.map((chunk) => Buffer.from(chunk))));
    }

    function onError(error) {
      finish(reject, badRequest(`request stream failed: ${error.message}`));
    }

    req.on('data', onData);
    req.on('end', onEnd);
    req.on('error', onError);
  });
}

export async function readJson(req, options = {}) {
  const headers = parseHeaders(req.headers);
  if (!isJson(headers)) {
    throw unsupportedMedia(headers['content-type'] ?? 'none');
  }
  const raw = await readBody(req, options);
  const text = raw.toString('utf8').trim();
  if (text.length === 0) {
    throw badRequest('a JSON body is required');
  }
  let parsed;
  try {
    parsed = JSON.parse(text);
  } catch (error) {
    throw badRequest(`the body is not valid JSON: ${error.message}`);
  }
  if (parsed === null || typeof parsed !== 'object' || Array.isArray(parsed)) {
    throw badRequest('the JSON body must be an object');
  }
  return parsed;
}

export async function readForm(req, options = {}) {
  const headers = parseHeaders(req.headers);
  if (!isForm(headers)) {
    throw unsupportedMedia(headers['content-type'] ?? 'none');
  }
  const raw = await readBody(req, options);
  const params = new URLSearchParams(raw.toString('utf8'));
  const out = Object.create(null);
  for (const key of params.keys()) {
    if (FORBIDDEN_KEYS.has(key)) continue;
    const values = params.getAll(key);
    out[key] = values.length > 1 ? values : values[0];
  }
  return out;
}

export function queryOf(url) {
  const out = Object.create(null);
  for (const key of url.searchParams.keys()) {
    if (FORBIDDEN_KEYS.has(key)) continue;
    const values = url.searchParams.getAll(key);
    out[key] = values.length > 1 ? values : values[0];
  }
  return out;
}

export function clientOf(req) {
  const headers = parseHeaders(req.headers);
  return {
    address: req.socket?.remoteAddress ?? 'unknown',
    forwarded: headers['x-forwarded-for'],
    agent: headers['user-agent'] ?? 'unknown',
  };
}

export function pick(object, fields) {
  const out = {};
  for (const field of fields) {
    if (Object.hasOwn(object, field)) out[field] = object[field];
  }
  return out;
}

export function requireFields(object, fields) {
  const missing = fields.filter((field) => object[field] === undefined);
  if (missing.length > 0) {
    throw badRequest(`missing field${missing.length === 1 ? '' : 's'}: ${missing.join(', ')}`);
  }
  return object;
}
