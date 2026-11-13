import { json } from './response.js';
import { createLogger } from '../util/log.js';

export function timing() {
  return async (ctx, next) => {
    const started = process.hrtime.bigint();
    await next();
    const ms = Number(process.hrtime.bigint() - started) / 1e6;
    ctx.timings = { ...(ctx.timings ?? {}), total: Math.round(ms * 100) / 100 };
    ctx.res.setHeader('x-response-time', String(ctx.timings.total));
  };
}

export function logging(logger = createLogger({ level: 'silent' })) {
  return async (ctx, next) => {
    logger.info('request', { method: ctx.method, path: ctx.path, client: ctx.client });
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
    if (['POST', 'PUT', 'PATCH'].includes(ctx.method)) {
      const type = ctx.headers['content-type'] ?? '';
      if (!type.startsWith('application/json')) {
        json(ctx.res, 415, { error: 'unsupported_media_type', message: 'send application/json' });
        return;
      }
    }
    await next();
  };
}

export function cors(origin = '*') {
  return async (ctx, next) => {
    ctx.res.setHeader('access-control-allow-origin', origin);
    ctx.res.setHeader('access-control-allow-headers', 'authorization, content-type');
    ctx.res.setHeader('access-control-allow-methods', 'GET, POST, PUT, PATCH, DELETE');
    if (ctx.method === 'OPTIONS') {
      ctx.res.writeHead(204);
      ctx.res.end();
      return;
    }
    await next();
  };
}

export function rateLimit({ limit = 120, windowMs = 60_000 } = {}) {
  const hits = new Map();
  return async (ctx, next) => {
    const key = `${ctx.client}:${ctx.path.split('/')[1] ?? ''}`;
    const now = Date.now();
    const bucket = hits.get(key);
    const next_state = bucket && now - bucket.start < windowMs ? { start: bucket.start, count: bucket.count + 1 } : { start: now, count: 1 };
    hits.set(key, next_state);
    if (next_state.count > limit) {
      json(ctx.res, 429, { error: 'rate_limited', message: `at most ${limit} requests per ${windowMs} ms` });
      return;
    }
    ctx.res.setHeader('x-ratelimit-remaining', String(Math.max(0, limit - next_state.count)));
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

export function healthProbe() {
  return async (ctx, next) => {
    if (ctx.path === '/health' && ctx.method === 'GET') {
      json(ctx.res, 200, { status: 'ok' });
      return;
    }
    await next();
  };
}
