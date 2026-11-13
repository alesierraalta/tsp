import { createHmac, timingSafeEqual } from 'node:crypto';
import { HttpError, unauthorized } from '../http/errors.js';

const CLOCK_SKEW_SECONDS = 30;

function base64url(value) {
  return Buffer.from(value).toString('base64url');
}

function sign(secret, body) {
  return createHmac('sha256', String(secret)).update(body).digest('base64url');
}

export function signToken(claims, secret, options = {}) {
  if (!secret) throw new TypeError('a signing secret is required');
  if (claims === null || typeof claims !== 'object') {
    throw new TypeError('token claims must be an object');
  }
  const now = options.now ?? Date.now();
  const iat = Math.floor(now / 1000);
  const ttl = options.ttlSeconds ?? 900;
  const payload = {
    ...claims,
    iat,
    exp: iat + Math.max(1, Math.floor(ttl)),
  };
  const body = base64url(JSON.stringify(payload));
  return `${body}.${sign(secret, body)}`;
}

export function decodeToken(token) {
  const parts = String(token ?? '').split('.');
  if (parts.length !== 2) {
    throw new HttpError(401, 'unauthorized', 'malformed token');
  }
  let payload;
  try {
    payload = JSON.parse(Buffer.from(parts[0], 'base64url').toString('utf8'));
  } catch {
    throw new HttpError(401, 'unauthorized', 'malformed token payload');
  }
  if (payload === null || typeof payload !== 'object') {
    throw new HttpError(401, 'unauthorized', 'malformed token payload');
  }
  return { body: parts[0], signature: parts[1], payload };
}

export function verifyToken(token, secret, options = {}) {
  if (!secret) throw new TypeError('a signing secret is required');
  const { body, signature, payload } = decodeToken(token);
  const expected = sign(secret, body);
  const given = Buffer.from(signature);
  const wanted = Buffer.from(expected);
  if (given.length !== wanted.length || !timingSafeEqual(given, wanted)) {
    throw unauthorized('token signature does not match');
  }
  const now = Math.floor((options.now ?? Date.now()) / 1000);
  if (typeof payload.exp !== 'number') {
    throw unauthorized('token has no expiry');
  }
  if (payload.exp + CLOCK_SKEW_SECONDS < now) {
    throw unauthorized('token has expired');
  }
  if (payload.iat !== undefined && payload.iat - CLOCK_SKEW_SECONDS > now) {
    throw unauthorized('token is not valid yet');
  }
  return payload;
}

export function refreshToken(token, secret, options = {}) {
  const claims = verifyToken(token, secret, options);
  const next = { ...claims };
  delete next.iat;
  delete next.exp;
  return signToken(next, secret, options);
}

export function tokenFingerprint(token) {
  return createHmac('sha256', 'fingerprint').update(String(token ?? '')).digest('hex').slice(0, 16);
}

export function ttlOf(token) {
  const { payload } = decodeToken(token);
  return payload.exp;
}
