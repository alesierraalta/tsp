import { createHmac, timingSafeEqual } from 'node:crypto';
import { maskSecret } from '../util/text.js';

function digest(secret, body) {
  return createHmac('sha256', String(secret)).update(body).digest('hex');
}

export function signBody(body, secret) {
  if (!secret) throw new TypeError('a signing secret is required');
  return digest(secret, String(body ?? ''));
}

export function signMessage(body, secret, timestamp) {
  const stamp = String(timestamp);
  const signature = digest(secret, `${stamp}.${String(body ?? '')}`);
  return `t=${stamp},v1=${signature}`;
}

export function parseSignatureHeader(header) {
  const parts = String(header ?? '')
    .split(',')
    .map((part) => part.trim())
    .filter(Boolean);
  const out = {};
  for (const part of parts) {
    const index = part.indexOf('=');
    if (index <= 0) continue;
    out[part.slice(0, index)] = part.slice(index + 1);
  }
  if (!out.t || !out.v1) throw new TypeError('a signature header needs t and v1');
  return { timestamp: Number(out.t), signature: out.v1 };
}

export function verifyMessage(body, header, secret, options = {}) {
  const tolerance = options.toleranceSeconds ?? 300;
  const now = Math.floor((options.now ?? Date.now()) / 1000);
  const { timestamp, signature } = parseSignatureHeader(header);
  if (!Number.isFinite(timestamp)) throw new TypeError('the signature timestamp is not a number');
  if (Math.abs(now - timestamp) > tolerance) {
    return { ok: false, reason: 'timestamp_out_of_tolerance' };
  }
  const expected = digest(secret, `${timestamp}.${String(body ?? '')}`);
  const given = Buffer.from(String(signature));
  const wanted = Buffer.from(expected);
  if (given.length !== wanted.length || !timingSafeEqual(given, wanted)) {
    return { ok: false, reason: 'signature_mismatch' };
  }
  return { ok: true, timestamp };
}

export function secretLabel(secret) {
  return maskSecret(String(secret ?? ''), 4);
}
