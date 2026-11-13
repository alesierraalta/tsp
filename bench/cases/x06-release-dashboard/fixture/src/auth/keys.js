import { createHash, randomBytes, timingSafeEqual } from 'node:crypto';
import { keyId } from '../util/ids.js';
import { maskSecret } from '../util/text.js';
import { conflict, unauthorized } from '../util/errors.js';

export const KEY_PREFIX = 'rdk_';
export const SCOPES = ['releases:read', 'releases:write', 'webhooks:write', 'admin'];

export function hashKey(secret) {
  return createHash('sha256').update(String(secret)).digest('hex');
}

export function generateKey() {
  return `${KEY_PREFIX}${randomBytes(18).toString('base64url')}`;
}

export function isKeyShaped(secret) {
  return new RegExp(`^${KEY_PREFIX}[A-Za-z0-9_-]{16,64}$`).test(String(secret ?? ''));
}

export function describeScopes(scopes) {
  const granted = new Set((scopes ?? []).filter((scope) => SCOPES.includes(scope)));
  return { scopes: SCOPES.map((scope) => ({ scope, granted: granted.has(scope) })), count: granted.size };
}

export class KeyRegistry {
  constructor(options = {}) {
    this.clock = options.clock ?? (() => new Date());
    this.keys = new Map();
    this.revoked = new Set();
  }

  issue(tenant, options = {}) {
    const secret = options.secret ?? generateKey();
    const scopes = Array.isArray(options.scopes) && options.scopes.length > 0 ? [...options.scopes] : ['releases:read'];
    const unknown = scopes.filter((scope) => !SCOPES.includes(scope));
    if (unknown.length > 0) throw conflict('unknown_scope', `${unknown[0]} is not a known scope`);
    const record = {
      id: keyId(),
      tenant,
      label: String(options.label ?? 'default'),
      scopes,
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

  rotate(id) {
    const found = [...this.keys.values()].find((record) => record.id === id);
    if (!found) throw conflict('key_missing', `key ${id} does not exist`);
    const secret = generateKey();
    const next = { ...found, hash: hashKey(secret), masked: maskSecret(secret), created_at: new Date(this.clock()).toISOString() };
    this.keys.delete(found.hash);
    this.keys.set(next.hash, next);
    return { ...next, secret };
  }

  list(tenant) {
    return [...this.keys.values()]
      .filter((record) => tenant === undefined || record.tenant === tenant)
      .map((record) => ({ id: record.id, tenant: record.tenant, label: record.label, masked: record.masked, scopes: record.scopes }));
  }

  expiring(days) {
    const horizon = new Date(this.clock()).getTime() + Math.max(0, Math.trunc(days)) * 86400000;
    return this.list().filter((record) => {
      const original = [...this.keys.values()].find((entry) => entry.id === record.id);
      return original?.expires_at !== undefined && original?.expires_at !== null && new Date(original.expires_at).getTime() <= horizon;
    });
  }

  hasScope(record, scope) {
    return (record?.scopes ?? []).includes(scope);
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

export function usageReport(records) {
  const byScope = {};
  for (const record of records ?? []) {
    for (const scope of record.scopes ?? []) byScope[scope] = (byScope[scope] ?? 0) + 1;
  }
  return {
    keys: (records ?? []).length,
    labels: [...new Set((records ?? []).map((record) => record.label))].length,
    byScope,
  };
}

export function withoutScope(records, scope) {
  return (records ?? []).filter((record) => !(record.scopes ?? []).includes(scope)).map((record) => record.id);
}

export function maskedList(records) {
  return (records ?? []).map((record) => ({ id: record.id, label: record.label, masked: record.masked }));
}

export function expiringSoon(records, days, clock = () => new Date()) {
  const horizon = new Date(clock()).getTime() + Math.max(0, Math.trunc(days)) * 86400000;
  return (records ?? []).filter((record) => record.expires_at !== null && record.expires_at !== undefined && new Date(record.expires_at).getTime() <= horizon);
}
