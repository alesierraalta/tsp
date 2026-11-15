import { randomToken } from '../util/ids.js';
import { maskSecret } from '../util/text.js';
import { notFound } from '../util/errors.js';

export function createKey(options = {}) {
  const id = options.id ?? `key_${randomToken(6)}`;
  const secret = options.secret ?? randomToken(24);
  if (String(secret).length < 16) throw new TypeError('a signing secret must be at least 16 characters');
  return {
    id,
    secret,
    label: options.label ?? 'default',
    created_at: new Date(options.now ?? Date.now()).toISOString(),
    active: options.active !== false,
  };
}

export function maskedKey(key) {
  return {
    id: key.id,
    label: key.label,
    active: key.active,
    created_at: key.created_at,
    secret: maskSecret(key.secret),
  };
}

export class KeyStore {
  constructor(options = {}) {
    this.clock = options.clock ?? (() => new Date());
    this.keys = new Map();
    if (options.seed) this.add(options.seed);
  }

  add(key) {
    if (this.keys.has(key.id)) throw new Error(`key ${key.id} already exists`);
    this.keys.set(key.id, key);
    return key;
  }

  create(options = {}) {
    return this.add(createKey({ now: this.clock(), ...options }));
  }

  get(id) {
    const key = this.keys.get(id);
    if (!key) throw notFound(`key ${id}`);
    return key;
  }

  active() {
    const key = [...this.keys.values()].find((entry) => entry.active);
    if (!key) throw notFound('an active signing key');
    return key;
  }

  rotate(label = 'rotated') {
    for (const key of this.keys.values()) key.active = false;
    return this.create({ label });
  }

  revoke(id) {
    const key = this.get(id);
    key.active = false;
    return key;
  }

  list() {
    return [...this.keys.values()].map((key) => maskedKey(key));
  }

  count() {
    return this.keys.size;
  }

  verifiers() {
    return [...this.keys.values()].filter((key) => key.active);
  }

  snapshot() {
    return [...this.keys.values()].map((key) => ({ ...key }));
  }
}
