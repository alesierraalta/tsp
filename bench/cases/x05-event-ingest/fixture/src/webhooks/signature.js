import { createHmac, timingSafeEqual } from 'node:crypto';

const TOLERANCE_SECONDS = 300;

export function signatureOf(payload, secret) {
  return createHmac('sha256', String(secret)).update(JSON.stringify(payload)).digest('hex');
}

export function headerValue(signature, timestamp) {
  return `t=${timestamp},v1=${signature}`;
}

export function parseSignature(header) {
  const parts = String(header ?? '')
    .split(',')
    .map((part) => part.split('='))
    .filter((pair) => pair.length === 2);
  const out = {};
  for (const [key, value] of parts) out[key.trim()] = value.trim();
  return out;
}

export function sign(payload, secret, options = {}) {
  const timestamp = Math.floor((options.now ?? Date.now()) / 1000);
  return headerValue(signatureOf(payload, secret), timestamp);
}

export function verify(payload, header, secret, options = {}) {
  const parsed = parseSignature(header);
  if (!parsed.t || !parsed.v1) return { ok: false, reason: 'a signature needs t and v1' };
  const timestamp = Number(parsed.t);
  if (!Number.isFinite(timestamp)) return { ok: false, reason: 't is not a number' };
  const now = Math.floor((options.now ?? Date.now()) / 1000);
  if (Math.abs(now - timestamp) > (options.tolerance ?? TOLERANCE_SECONDS)) {
    return { ok: false, reason: 'the signature is outside the tolerance' };
  }
  const expected = Buffer.from(signatureOf(payload, secret));
  const provided = Buffer.from(parsed.v1);
  if (expected.length !== provided.length || !timingSafeEqual(expected, provided)) {
    return { ok: false, reason: 'the signature does not match' };
  }
  return { ok: true, timestamp };
}

export function signOutgoing(subscription, payload, options = {}) {
  const secret = subscription?.secret ?? options.secret ?? 'development-secret';
  return { header: sign(payload, secret, options), body: payload };
}

export function signatureAge(header, options = {}) {
  const parsed = parseSignature(header);
  const timestamp = Number(parsed.t);
  if (!Number.isFinite(timestamp)) return undefined;
  return Math.abs(Math.floor((options.now ?? Date.now()) / 1000) - timestamp);
}
