import { createHmac, timingSafeEqual } from 'node:crypto';
import { unauthorized } from '../util/errors.js';

const SKEW_SECONDS = 30;

function base64url(input) {
  return Buffer.from(input).toString('base64url');
}

function decode(text) {
  return Buffer.from(String(text), 'base64url').toString('utf8');
}

export function signPayload(payload, secret) {
  const body = base64url(JSON.stringify(payload));
  const signature = createHmac('sha256', String(secret)).update(body).digest('base64url');
  return `${body}.${signature}`;
}

export function signToken(payload, secret) {
  return signPayload({ ...payload, exp: payload.exp ?? Math.floor(Date.now() / 1000) + 3600 }, secret);
}

export function verifyToken(token, secret, options = {}) {
  const text = String(token ?? '').trim();
  if (text === '') throw unauthorized('a token is required');
  const parts = text.split('.');
  if (parts.length !== 2) throw unauthorized('a token is malformed');
  const [body, signature] = parts;
  const expected = createHmac('sha256', String(secret)).update(body).digest('base64url');
  const left = Buffer.from(signature);
  const right = Buffer.from(expected);
  if (left.length !== right.length || !timingSafeEqual(left, right)) {
    throw unauthorized('the token signature does not match');
  }
  let payload;
  try {
    payload = JSON.parse(decode(body));
  } catch {
    throw unauthorized('the token payload is not json');
  }
  const now = Math.floor((options.now ?? Date.now()) / 1000);
  if (typeof payload.exp !== 'number') throw unauthorized('the token has no expiry');
  if (payload.exp + SKEW_SECONDS < now) throw unauthorized('the token has expired');
  return payload;
}

export function decodeUnsafe(token) {
  const parts = String(token ?? '').split('.');
  if (parts.length !== 2) return undefined;
  try {
    return JSON.parse(decode(parts[0]));
  } catch {
    return undefined;
  }
}

export const SCOPES = ['content:read', 'content:write', 'media:write', 'admin', 'export:read'];

export function hasScope(payload, scope) {
  return Array.isArray(payload?.scopes) && payload.scopes.includes(scope);
}

export function assertScope(payload, scope) {
  if (!hasScope(payload, scope)) throw unauthorized(`this token lacks ${scope}`);
  return true;
}

export function refreshToken(token, secret, options = {}) {
  const payload = verifyToken(token, secret, options);
  const { exp: _exp, iat: _iat, ...rest } = payload;
  return signToken({ ...rest, refreshed_at: Math.floor(Date.now() / 1000) }, secret);
}

export function tokenLifetime(token) {
  const payload = decodeUnsafe(token);
  if (!payload || typeof payload.exp !== 'number') return undefined;
  return Math.max(0, payload.exp - Math.floor(Date.now() / 1000));
}

export function tokenSummary(token) {
  const payload = decodeUnsafe(token);
  if (!payload) return { valid: false };
  return {
    valid: true,
    subject: payload.sub,
    tenant: payload.tenant,
    scopes: Array.isArray(payload.scopes) ? payload.scopes : [],
    expires_in: tokenLifetime(token),
  };
}
