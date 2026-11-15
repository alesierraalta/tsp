import { conflict, notFound } from '../http/errors.js';
import { newId } from '../util/ids.js';

function stamp(row, clock) {
  const now = new Date(clock()).toISOString();
  if (row.created_at === undefined) row.created_at = now;
  row.updated_at = now;
}

export class MemoryStore {
  constructor(options = {}) {
    this.clock = options.clock ?? (() => new Date());
    this.tables = new Map();
    this.pending = Promise.resolve();
    this.writes = 0;
  }

  table(name) {
    let rows = this.tables.get(name);
    if (!rows) {
      rows = new Map();
      this.tables.set(name, rows);
    }
    return rows;
  }

  names() {
    return [...this.tables.keys()].sort();
  }

  count(name) {
    return this.table(name).size;
  }

  has(name, id) {
    return this.table(name).has(id);
  }

  get(name, id) {
    return this.table(name).get(id);
  }

  insert(name, row) {
    if (row === null || typeof row !== 'object') {
      throw new TypeError('a row object is required');
    }
    const id = row.id ?? newId(name.slice(0, 3));
    if (this.has(name, id)) {
      throw conflict(`${name} ${id} already exists`);
    }
    const stored = { ...row, id };
    stamp(stored, this.clock);
    this.table(name).set(id, stored);
    this.writes += 1;
    return stored;
  }

  update(name, id, mutator) {
    const row = this.get(name, id);
    if (!row) throw notFound(name);
    if (typeof mutator !== 'function') throw new TypeError('an update function is required');
    const next = mutator(row) ?? row;
    if (next.id !== undefined && next.id !== id) {
      throw conflict('a row id cannot be changed');
    }
    stamp(next, this.clock);
    this.table(name).set(id, next);
    this.writes += 1;
    return next;
  }

  remove(name, id) {
    const removed = this.table(name).delete(id);
    if (!removed) throw notFound(name);
    this.writes += 1;
    return removed;
  }

  all(name) {
    return [...this.table(name).values()];
  }

  find(name, predicate) {
    return this.all(name).filter(predicate);
  }

  first(name, predicate) {
    return this.all(name).find(predicate);
  }

  clear(name) {
    if (name === undefined) {
      this.tables.clear();
      return;
    }
    this.table(name).clear();
  }

  async transaction(fn) {
    const run = this.pending.then(() => fn(this));
    this.pending = run.then(
      () => undefined,
      () => undefined,
    );
    return run;
  }

  snapshot() {
    const out = {};
    for (const [name, rows] of this.tables.entries()) {
      out[name] = [...rows.values()];
    }
    return out;
  }
}
