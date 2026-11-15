export class DedupeWindow {
  constructor(options = {}) {
    this.limit = options.limit ?? 50000;
    this.seenIds = new Map();
  }

  keyOf(tenant, id) {
    return `${tenant}|${id}`;
  }

  seen(tenant, id) {
    return this.seenIds.has(this.keyOf(tenant, id));
  }

  remember(tenant, id) {
    const key = this.keyOf(tenant, id);
    if (this.seenIds.has(key)) return false;
    this.seenIds.set(key, Date.now());
    if (this.seenIds.size > this.limit) {
      const excess = this.seenIds.size - this.limit;
      let removed = 0;
      for (const candidate of this.seenIds.keys()) {
        this.seenIds.delete(candidate);
        removed += 1;
        if (removed >= excess) break;
      }
    }
    return true;
  }

  forget(tenant, id) {
    return this.seenIds.delete(this.keyOf(tenant, id));
  }

  size() {
    return this.seenIds.size;
  }

  clear() {
    const size = this.seenIds.size;
    this.seenIds.clear();
    return size;
  }
}

export function dedupeEvents(events, keyOf = (event) => event.id) {
  const seen = new Set();
  const out = [];
  const duplicates = [];
  for (const event of events ?? []) {
    const key = keyOf(event);
    if (seen.has(key)) {
      duplicates.push(key);
      continue;
    }
    seen.add(key);
    out.push(event);
  }
  return { events: out, duplicates };
}

export function collisionRate(events, keyOf = (event) => event.id) {
  const { duplicates } = dedupeEvents(events, keyOf);
  return events.length === 0 ? 0 : Math.round((duplicates.length / events.length) * 10000) / 100;
}

export function windowStats(window) {
  const buckets = {};
  for (const label of [...window.seenIds.keys()]) {
    const tenant = label.split('|')[0];
    buckets[tenant] = (buckets[tenant] ?? 0) + 1;
  }
  return { size: window.size(), limit: window.limit, byTenant: buckets };
}

export function evictTenant(window, tenant) {
  let removed = 0;
  for (const label of [...window.seenIds.keys()]) {
    if (!label.startsWith(`${tenant}|`)) continue;
    window.seenIds.delete(label);
    removed += 1;
  }
  return removed;
}

export function overlap(left, right, keyOf = (event) => event.id) {
  const keys = new Set((right ?? []).map(keyOf));
  const shared = (left ?? []).map(keyOf).filter((label) => keys.has(label));
  return { shared: shared.length, unique: new Set(shared).size, keys: [...new Set(shared)] };
}

export function dedupeReport(events, keyOf) {
  const { events: kept, duplicates } = dedupeEvents(events, keyOf);
  return {
    received: (events ?? []).length,
    kept: kept.length,
    duplicates: duplicates.length,
    rate: collisionRate(events ?? [], keyOf),
  };
}
