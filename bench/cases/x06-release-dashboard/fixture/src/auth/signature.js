import { createHmac, timingSafeEqual } from 'node:crypto';

export const TOLERANCE_SECONDS = 300;

export function signatureOf(body, secret) {
  return createHmac('sha256', String(secret)).update(String(body)).digest('hex');
}

export function headerFor(body, secret, now = Date.now()) {
  const timestamp = Math.floor(now / 1000);
  return `t=${timestamp},v1=${signatureOf(`${timestamp}.${body}`, secret)}`;
}

export function parseHeader(header) {
  const parts = String(header ?? '')
    .split(',')
    .map((part) => part.split('='))
    .filter((pair) => pair.length === 2);
  const out = {};
  for (const [name, value] of parts) out[name.trim()] = value.trim();
  return out;
}

export function verifyDelivery(body, header, secret, options = {}) {
  const parsed = parseHeader(header);
  if (!parsed.t || !parsed.v1) return { ok: false, reason: 'a signature needs t and v1' };
  const timestamp = Number(parsed.t);
  if (!Number.isFinite(timestamp)) return { ok: false, reason: 't is not a number' };
  const now = Math.floor((options.now ?? Date.now()) / 1000);
  if (Math.abs(now - timestamp) > (options.tolerance ?? TOLERANCE_SECONDS)) {
    return { ok: false, reason: 'the signature is outside the tolerance' };
  }
  const expected = Buffer.from(signatureOf(`${timestamp}.${body}`, secret));
  const provided = Buffer.from(parsed.v1);
  if (expected.length !== provided.length || !timingSafeEqual(expected, provided)) {
    return { ok: false, reason: 'the signature does not match' };
  }
  return { ok: true, timestamp };
}

export function signatureAge(header, options = {}) {
  const parsed = parseHeader(header);
  const timestamp = Number(parsed.t);
  if (!Number.isFinite(timestamp)) return undefined;
  return Math.abs(Math.floor((options.now ?? Date.now()) / 1000) - timestamp);
}

export function signPayload(payload, secret, now = Date.now()) {
  const body = typeof payload === 'string' ? payload : JSON.stringify(payload ?? {});
  return { header: headerFor(body, secret, now), body };
}
