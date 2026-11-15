import { clone } from '../util/json.js';
import { notFound } from '../util/errors.js';

export class MemoryStore {
  constructor(options = {}) {
    this.clock = options.clock ?? (() => new Date());
    this.collections = new Map();
    this.sequence = 0;
    this.writes = 0;
  }

  bucket(name) {
    const key = String(name);
    let rows = this.collections.get(key);
    if (!rows) {
      rows = new Map();
      this.collections.set(key, rows);
    }
    return rows;
  }

  insert(name, id, value) {
    const rows = this.bucket(name);
    if (rows.has(id)) throw new Error(`${name} ${id} already exists`);
    rows.set(id, clone(value));
    this.writes += 1;
    this.sequence += 1;
    return clone(value);
  }

  get(name, id) {
    const row = this.bucket(name).get(id);
    return row === undefined ? undefined : clone(row);
  }

  require(name, id) {
    const row = this.get(name, id);
    if (row === undefined) throw notFound(`${String(name).replace(/s$/, '')} ${id}`);
    return row;
  }

  has(name, id) {
    return this.bucket(name).has(id);
  }

  update(name, id, patch) {
    const rows = this.bucket(name);
    const current = rows.get(id);
    if (current === undefined) throw notFound(`${name} ${id}`);
    const next = { ...clone(current), ...clone(patch) };
    rows.set(id, next);
    this.writes += 1;
    this.sequence += 1;
    return clone(next);
  }

  replace(name, id, value) {
    const rows = this.bucket(name);
    if (!rows.has(id)) throw notFound(`${name} ${id}`);
    rows.set(id, clone(value));
    this.writes += 1;
    return clone(value);
  }

  remove(name, id) {
    const rows = this.bucket(name);
    const current = rows.get(id);
    if (current === undefined) return false;
    rows.delete(id);
    this.writes += 1;
    return true;
  }

  all(name) {
    return [...this.bucket(name).values()].map((row) => clone(row));
  }

  values(name) {
    return [...this.bucket(name).values()];
  }

  ids(name) {
    return [...this.bucket(name).keys()];
  }

  count(name) {
    return this.bucket(name).size;
  }

  find(name, predicate) {
    return this.all(name).filter(predicate);
  }

  findOne(name, predicate) {
    for (const row of this.bucket(name).values()) {
      if (predicate(row)) return clone(row);
    }
    return undefined;
  }

  clear(name) {
    this.bucket(name).clear();
    this.writes += 1;
  }

  snapshot() {
    const out = {};
    for (const [name, rows] of this.collections.entries()) {
      out[name] = Object.fromEntries([...rows.entries()].map(([id, row]) => [id, clone(row)]));
    }
    return out;
  }

  restore(snapshot) {
    this.collections = new Map();
    for (const [name, rows] of Object.entries(snapshot ?? {})) {
      const bucket = new Map();
      for (const [id, row] of Object.entries(rows)) bucket.set(id, clone(row));
      this.collections.set(name, bucket);
    }
    this.writes += 1;
  }

  stats() {
    const out = {};
    for (const [name, rows] of this.collections.entries()) out[name] = rows.size;
    return { collections: out, writes: this.writes };
  }

  first(name, predicate) {
    for (const row of this.bucket(name).values()) {
      if (predicate(row)) return clone(row);
    }
    return undefined;
  }

  latest(name, path = 'created_at') {
    let best;
    let bestAt;
    for (const row of this.bucket(name).values()) {
      const at = row[path];
      if (bestAt === undefined || at > bestAt) {
        best = row;
        bestAt = at;
      }
    }
    return best === undefined ? undefined : clone(best);
  }

  map(name, transform) {
    return this.all(name).map(transform);
  }

  reduce(name, step, initial) {
    let acc = initial;
    for (const row of this.bucket(name).values()) acc = step(acc, clone(row));
    return acc;
  }

  upsert(name, id, value) {
    return this.has(name, id) ? this.replace(name, id, value) : this.insert(name, id, value);
  }

  transfer(from, to) {
    const source = this.bucket(from);
    const target = this.bucket(to);
    let moved = 0;
    for (const [id, row] of source.entries()) {
      target.set(id, clone(row));
      moved += 1;
    }
    source.clear();
    this.writes += 1;
    return moved;
  }

  destroy() {
    const collections = this.collections.size;
    this.collections.clear();
    this.sequence = 0;
    return collections;
  }
}
