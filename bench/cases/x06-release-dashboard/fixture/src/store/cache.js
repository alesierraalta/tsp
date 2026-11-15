export class Cache {
  constructor(options = {}) {
    this.clock = options.clock ?? (() => new Date());
    this.ttlMs = options.ttlMs ?? 30000;
    this.limit = options.limit ?? 500;
    this.entries = new Map();
    this.hits = 0;
    this.misses = 0;
  }

  keyOf(namespace, parts = {}) {
    const suffix = Object.entries(parts)
      .filter(([, value]) => value !== undefined)
      .sort(([left], [right]) => (left < right ? -1 : 1))
      .map(([name, value]) => `${name}=${value}`)
      .join('&');
    return suffix === '' ? String(namespace) : `${namespace}?${suffix}`;
  }

  fresh(entry) {
    return entry !== undefined && new Date(entry.expires_at).getTime() > new Date(this.clock()).getTime();
  }

  get(namespace, parts) {
    const name = this.keyOf(namespace, parts);
    const entry = this.entries.get(name);
    if (!this.fresh(entry)) {
      this.misses += 1;
      return undefined;
    }
    this.hits += 1;
    return entry.value;
  }

  set(namespace, parts, value, ttlMs) {
    const name = this.keyOf(namespace, parts);
    const expires = new Date(this.clock()).getTime() + Math.max(0, Math.trunc(ttlMs ?? this.ttlMs));
    this.entries.set(name, { value, stored_at: new Date(this.clock()).toISOString(), expires_at: new Date(expires).toISOString() });
    this.trim();
    return value;
  }

  remember(namespace, parts, producer, ttlMs) {
    const cached = this.get(namespace, parts);
    if (cached !== undefined) return cached;
    const value = producer();
    this.set(namespace, parts, value, ttlMs);
    return value;
  }

  delete(namespace, parts) {
    return this.entries.delete(this.keyOf(namespace, parts));
  }

  clear(namespace) {
    if (namespace === undefined) {
      const size = this.entries.size;
      this.entries.clear();
      return size;
    }
    let removed = 0;
    for (const name of [...this.entries.keys()]) {
      if (!name.startsWith(String(namespace))) continue;
      this.entries.delete(name);
      removed += 1;
    }
    return removed;
  }

  trim() {
    while (this.entries.size > this.limit) {
      const oldest = [...this.entries.entries()].sort(([, left], [, right]) => (left.stored_at < right.stored_at ? -1 : 1))[0];
      if (oldest === undefined) break;
      this.entries.delete(oldest[0]);
    }
    return this.entries.size;
  }

  prune() {
    let removed = 0;
    for (const [name, entry] of [...this.entries.entries()]) {
      if (this.fresh(entry)) continue;
      this.entries.delete(name);
      removed += 1;
    }
    return removed;
  }

  stats() {
    return { entries: this.entries.size, hits: this.hits, misses: this.misses, hit_rate: this.hits + this.misses === 0 ? 0 : Math.round((this.hits / (this.hits + this.misses)) * 100) };
  }
}

export function cachedDashboard(cache, tenant, environment, producer) {
  return cache.remember('dashboard', { tenant, environment }, producer);
}

export function invalidateTenant(cache, tenant) {
  let removed = 0;
  for (const name of [...cache.entries.keys()]) {
    if (!name.includes(`tenant=${tenant}`)) continue;
    cache.entries.delete(name);
    removed += 1;
  }
  return removed;
}
