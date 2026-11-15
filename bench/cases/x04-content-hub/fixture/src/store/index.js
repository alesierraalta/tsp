import { readPath } from './query.js';

export class SecondaryIndex {
  constructor(name, path, options = {}) {
    this.name = name;
    this.path = path;
    this.multi = options.multi === true;
    this.buckets = new Map();
  }

  add(row) {
    const value = readPath(row, this.path);
    if (value === undefined || value === null) return;
    const keys = this.multi && Array.isArray(value) ? value : [value];
    for (const key of keys) {
      const bucket = this.buckets.get(key);
      if (bucket) bucket.add(row.id);
      else this.buckets.set(key, new Set([row.id]));
    }
  }

  remove(row) {
    const value = readPath(row, this.path);
    if (value === undefined || value === null) return;
    const keys = this.multi && Array.isArray(value) ? value : [value];
    for (const key of keys) {
      const bucket = this.buckets.get(key);
      if (!bucket) continue;
      bucket.delete(row.id);
      if (bucket.size === 0) this.buckets.delete(key);
    }
  }

  lookup(value) {
    return [...(this.buckets.get(value) ?? [])];
  }

  keys() {
    return [...this.buckets.keys()];
  }

  size() {
    return this.buckets.size;
  }
}

export class IndexSet {
  constructor(rows = []) {
    this.indexes = new Map();
    for (const row of rows) this.add(row);
  }

  define(name, path, options = {}) {
    const index = new SecondaryIndex(name, path, options);
    this.indexes.set(name, index);
    for (const row of options.rows ?? []) index.add(row);
    return index;
  }

  get(name) {
    const index = this.indexes.get(name);
    if (!index) throw new TypeError(`no index named ${name}`);
    return index;
  }

  add(row) {
    for (const index of this.indexes.values()) index.add(row);
  }

  remove(row) {
    for (const index of this.indexes.values()) index.remove(row);
  }

  rebuild(rows) {
    for (const index of this.indexes.values()) index.buckets.clear();
    for (const row of rows) this.add(row);
  }

  stats() {
    const out = {};
    for (const [name, index] of this.indexes.entries()) out[name] = index.size();
    return out;
  }
}

export function buildIndexes(rows, definitions) {
  const set = new IndexSet(rows);
  for (const [name, definition] of Object.entries(definitions)) {
    const index = new SecondaryIndex(name, definition.path, definition);
    for (const row of rows) index.add(row);
    set.indexes.set(name, index);
  }
  return set;
}
