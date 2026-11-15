import { createHash, randomBytes, timingSafeEqual } from 'node:crypto';
import { keyId } from '../util/ids.js';
import { maskSecret } from '../util/text.js';
import { conflict, unauthorized } from '../util/errors.js';

export const KEY_PREFIX = 'ing_';

export function hashKey(secret) {
  return createHash('sha256').update(String(secret)).digest('hex');
}

export function generateKey() {
  return `${KEY_PREFIX}${randomBytes(18).toString('base64url')}`;
}

export function isKeyShaped(secret) {
  return new RegExp(`^${KEY_PREFIX}[A-Za-z0-9_-]{16,64}$`).test(String(secret ?? ''));
}

export class KeyRegistry {
  constructor(options = {}) {
    this.clock = options.clock ?? (() => new Date());
    this.keys = new Map();
    this.revoked = new Set();
  }

  issue(tenant, options = {}) {
    const secret = options.secret ?? generateKey();
    const record = {
      id: keyId(),
      tenant,
      label: String(options.label ?? 'default'),
      scopes: Array.isArray(options.scopes) && options.scopes.length > 0 ? [...options.scopes] : ['ingest:write'],
      hash: hashKey(secret),
      masked: maskSecret(secret),
      created_at: new Date(this.clock()).toISOString(),
      expires_at: options.expires_at ?? null,
    };
    this.keys.set(record.hash, record);
    return { ...record, secret };
  }

  resolve(secret) {
    if (!isKeyShaped(secret)) throw unauthorized('the api key is malformed');
    const record = this.keys.get(hashKey(secret));
    if (!record) throw unauthorized('the api key is unknown');
    if (this.revoked.has(record.id)) throw unauthorized('the api key was revoked');
    if (record.expires_at && new Date(record.expires_at).getTime() <= new Date(this.clock()).getTime()) {
      throw unauthorized('the api key has expired');
    }
    return record;
  }

  revoke(id) {
    const found = [...this.keys.values()].find((record) => record.id === id);
    if (!found) return false;
    this.revoked.add(found.id);
    return true;
  }

  list(tenant) {
    return [...this.keys.values()]
      .filter((record) => tenant === undefined || record.tenant === tenant)
      .map((record) => ({ id: record.id, tenant: record.tenant, label: record.label, masked: record.masked }));
  }

  hasScope(record, scope) {
    return (record.scopes ?? []).includes(scope);
  }

  assertScope(record, scope) {
    if (!this.hasScope(record, scope)) throw unauthorized(`this key lacks ${scope}`);
    return true;
  }

  rotate(id) {
    const found = [...this.keys.values()].find((record) => record.id === id);
    if (!found) throw conflict('key_missing', `key ${id} does not exist`);
    const secret = generateKey();
    const next = { ...found, hash: hashKey(secret), masked: maskSecret(secret), created_at: new Date(this.clock()).toISOString() };
    this.keys.delete(found.hash);
    this.keys.set(next.hash, next);
    return { ...next, secret };
  }

  stats() {
    return { keys: this.keys.size, revoked: this.revoked.size };
  }
}

export function constantTimeEquals(left, right) {
  const a = Buffer.from(String(left));
  const b = Buffer.from(String(right));
  return a.length === b.length && timingSafeEqual(a, b);
}

export const SCOPES = ['reports:read', 'ingest:write', 'admin'];

export function scopeSet(scopes) {
  return new Set((scopes ?? []).filter((scope) => SCOPES.includes(scope)));
}

export function describeScopes(scopes) {
  const set = scopeSet(scopes);
  return { scopes: SCOPES.map((scope) => ({ scope, granted: set.has(scope) })), count: set.size };
}

export function assertKnownScopes(scopes) {
  const unknown = (scopes ?? []).filter((scope) => !SCOPES.includes(scope));
  if (unknown.length > 0) throw conflict('unknown_scope', `${unknown[0]} is not a known scope`);
  return scopes;
}

export function expiresSoon(record, days, clock = () => new Date()) {
  if (!record?.expires_at) return false;
  const remaining = new Date(record.expires_at).getTime() - new Date(clock()).getTime();
  return remaining > 0 && remaining <= Math.max(0, Math.trunc(days)) * 86400000;
}

export class ScopeReport {
  constructor(records = []) {
    this.records = records;
  }

  byScope() {
    const out = {};
    for (const record of this.records) {
      for (const scope of scopeSet(record.scopes)) out[scope] = (out[scope] ?? 0) + 1;
    }
    return out;
  }

  withoutScope(scope) {
    return this.records.filter((record) => !scopeSet(record.scopes).has(scope)).map((record) => record.id);
  }

  report() {
    return { keys: this.records.length, scopes: this.byScope() };
  }
}
