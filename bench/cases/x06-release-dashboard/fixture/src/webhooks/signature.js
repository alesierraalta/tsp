import { createHmac, timingSafeEqual } from 'node:crypto';

export const SIGNATURE_HEADER = 'x-dashboard-signature';
export const TOLERANCE_SECONDS = 300;

export function headerName() {
  return SIGNATURE_HEADER;
}

export function signatureOf(body, secret) {
  return createHmac('sha256', String(secret)).update(String(body)).digest('hex');
}

export function signDelivery(body, secret, now = Date.now()) {
  const timestamp = Math.floor(now / 1000);
  return `t=${timestamp},v1=${signatureOf(`${timestamp}.${body}`, secret)}`;
}

export function parseSignature(header) {
  const parts = String(header ?? '')
    .split(',')
    .map((part) => part.split('='))
    .filter((pair) => pair.length === 2);
  const out = {};
  for (const [name, value] of parts) out[name.trim()] = value.trim();
  return out;
}

export function verifySignature(body, header, secret, options = {}) {
  const parsed = parseSignature(header);
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
  const parsed = parseSignature(header);
  const timestamp = Number(parsed.t);
  if (!Number.isFinite(timestamp)) return undefined;
  return Math.abs(Math.floor((options.now ?? Date.now()) / 1000) - timestamp);
}

export class SecretBook {
  constructor(options = {}) {
    this.secrets = new Map();
    this.prefix = options.prefix ?? 'whsec';
  }

  issue(subscriptionId, secret) {
    const value = secret ?? `${this.prefix}_${signatureOf(subscriptionId, 'seed').slice(0, 32)}`;
    this.secrets.set(String(subscriptionId), value);
    return { subscription: String(subscriptionId), secret: value, masked: `${value.slice(0, 8)}…` };
  }

  secretFor(subscriptionId) {
    return this.secrets.get(String(subscriptionId));
  }

  rotate(subscriptionId) {
    const previous = this.secretFor(subscriptionId);
    if (previous === undefined) return undefined;
    return this.issue(subscriptionId, `${this.prefix}_${signatureOf(`${subscriptionId}${previous}`, 'rotate').slice(0, 32)}`);
  }

  forget(subscriptionId) {
    return this.secrets.delete(String(subscriptionId));
  }

  stats() {
    return { secrets: this.secrets.size };
  }
}
