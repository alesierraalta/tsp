import { json } from './response.js';
import { createLogger } from '../util/log.js';
import { contentTypeOf } from './headers.js';

export function timing() {
  return async (ctx, next) => {
    const started = process.hrtime.bigint();
    await next();
    const ms = Math.round((Number(process.hrtime.bigint() - started) / 1e6) * 100) / 100;
    if (!ctx.res.headersSent) ctx.res.setHeader('x-response-time', String(ms));
    ctx.durationMs = ms;
  };
}

export function logging(logger = createLogger({ level: 'silent' })) {
  return async (ctx, next) => {
    logger.info('request', { method: ctx.method, path: ctx.path });
    try {
      await next();
    } catch (error) {
      logger.error('request failed', { method: ctx.method, path: ctx.path, message: error.message });
      throw error;
    }
  };
}

export function requiresJson() {
  return async (ctx, next) => {
    if (!['POST', 'PUT', 'PATCH'].includes(ctx.method)) {
      await next();
      return;
    }
    const type = contentTypeOf(ctx.headers);
    if (type !== 'application/json' && type !== '') {
      json(ctx.res, 415, { error: 'unsupported_media_type', message: 'send application/json' });
      return;
    }
    await next();
  };
}

export function wantsHtml() {
  return async (ctx, next) => {
    if (ctx.method !== 'GET') {
      await next();
      return;
    }
    const accept = String(ctx.headers?.accept ?? '');
    ctx.wantsHtml = accept.includes('text/html') && String(ctx.query.format ?? '') !== 'json';
    await next();
  };
}

export function rateLimit(options = {}) {
  const limit = options.limit ?? 300;
  const windowMs = options.windowMs ?? 60000;
  const hits = new Map();
  return async (ctx, next) => {
    const key = ctx.client ?? 'unknown';
    const now = Date.now();
    const bucket = hits.get(key);
    const state = bucket && now - bucket.start < windowMs ? { start: bucket.start, count: bucket.count + 1 } : { start: now, count: 1 };
    hits.set(key, state);
    if (state.count > limit) {
      json(ctx.res, 429, { error: 'rate_limited', message: `at most ${limit} requests per ${windowMs} ms` });
      return;
    }
    if (!ctx.res.headersSent) ctx.res.setHeader('x-ratelimit-remaining', String(Math.max(0, limit - state.count)));
    await next();
  };
}

export function cors(origin = '*') {
  return async (ctx, next) => {
    ctx.res.setHeader('access-control-allow-origin', origin);
    ctx.res.setHeader('access-control-allow-headers', 'authorization, content-type, x-api-key');
    ctx.res.setHeader('access-control-allow-methods', 'GET, POST, PUT, PATCH, DELETE');
    if (ctx.method === 'OPTIONS') {
      ctx.res.writeHead(204);
      ctx.res.end();
      return;
    }
    await next();
  };
}

export function securityHeaders() {
  return async (ctx, next) => {
    ctx.res.setHeader('referrer-policy', 'no-referrer');
    ctx.res.setHeader('x-frame-options', 'deny');
    await next();
  };
}

export function compose(...layers) {
  return async (ctx, next) => {
    const stack = [...layers];
    let index = 0;
    const run = async () => {
      if (index >= stack.length) return next();
      const layer = stack[index];
      index += 1;
      return layer(ctx, run);
    };
    return run();
  };
}

export function attachPrincipal(resolve) {
  return async (ctx, next) => {
    try {
      ctx.principal = resolve(ctx);
    } catch {
      ctx.principal = undefined;
    }
    await next();
  };
}

export function bodyLimit(limit) {
  return async (ctx, next) => {
    const declared = Number(ctx.headers?.['content-length'] ?? 0);
    if (Number.isFinite(declared) && declared > limit) {
      json(ctx.res, 413, { error: 'payload_too_large', message: `a body holds at most ${limit} bytes` });
      return;
    }
    await next();
  };
}

export function requestId() {
  return async (ctx, next) => {
    if (ctx.requestId && !ctx.res.headersSent) ctx.res.setHeader('x-request-id', ctx.requestId);
    await next();
  };
}

export function noStore() {
  return async (ctx, next) => {
    if (!ctx.res.headersSent) ctx.res.setHeader('cache-control', 'no-store');
    await next();
  };
}

export function onlyMethods(methods) {
  const allowed = new Set((methods ?? []).map((method) => String(method).toUpperCase()));
  return async (ctx, next) => {
    if (!allowed.has(ctx.method)) {
      json(ctx.res, 405, { error: 'method_not_allowed', message: `${ctx.method} is not allowed here` });
      return;
    }
    await next();
  };
}
