import { clone } from '../util/json.js';
import { notFound } from '../util/errors.js';

export class MemoryStore {
  constructor(options = {}) {
    this.clock = options.clock ?? (() => new Date());
    this.collections = new Map();
    this.writes = 0;
  }

  bucket(name) {
    let rows = this.collections.get(name);
    if (!rows) {
      rows = new Map();
      this.collections.set(name, rows);
    }
    return rows;
  }

  insert(name, id, value) {
    const rows = this.bucket(name);
    if (rows.has(id)) throw new Error(`${name} ${id} already exists`);
    rows.set(id, clone(value));
    this.writes += 1;
    return clone(value);
  }

  get(name, id) {
    const row = this.bucket(name).get(id);
    return row === undefined ? undefined : clone(row);
  }

  require(name, id) {
    const row = this.get(name, id);
    if (row === undefined) throw notFound(`${name.replace(/s$/, '')} ${id}`);
    return row;
  }

  has(name, id) {
    return this.bucket(name).has(id);
  }

  replace(name, id, value) {
    const rows = this.bucket(name);
    if (!rows.has(id)) throw notFound(`${name} ${id}`);
    rows.set(id, clone(value));
    this.writes += 1;
    return clone(value);
  }

  update(name, id, patch) {
    const current = this.get(name, id);
    if (current === undefined) throw notFound(`${name} ${id}`);
    return this.replace(name, id, { ...current, ...clone(patch) });
  }

  remove(name, id) {
    const removed = this.bucket(name).delete(id);
    if (removed) this.writes += 1;
    return removed;
  }

  all(name) {
    return [...this.bucket(name).values()].map((row) => clone(row));
  }

  raw(name) {
    return [...this.bucket(name).values()];
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
      this.collections.set(name, new Map(Object.entries(clone(rows))));
    }
    this.writes += 1;
  }

  stats() {
    const out = {};
    for (const [name, rows] of this.collections.entries()) out[name] = rows.size;
    return { collections: out, writes: this.writes };
  }
}
