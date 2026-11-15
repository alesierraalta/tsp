import { clone } from '../util/json.js';
import { conflict } from '../util/errors.js';

export class Transaction {
  constructor(store, options = {}) {
    this.store = store;
    this.clock = options.clock ?? (() => new Date());
    this.pending = new Map();
    this.reads = new Set();
    this.state = 'open';
    this.startedAt = new Date(this.clock()).toISOString();
  }

  assertOpen() {
    if (this.state !== 'open') throw conflict('transaction_closed', `this transaction is ${this.state}`);
    return true;
  }

  read(collection, id) {
    this.assertOpen();
    const buffered = this.pending.get(`${collection}|${id}`);
    this.reads.add(`${collection}|${id}`);
    if (buffered !== undefined) return buffered === null ? undefined : clone(buffered);
    return this.store.get(collection, id);
  }

  all(collection) {
    this.assertOpen();
    const base = this.store.all(collection);
    const overrides = new Map();
    const removed = new Set();
    for (const [name, value] of this.pending.entries()) {
      const [owner, id] = name.split('|');
      if (owner !== collection) continue;
      if (value === null) removed.add(id);
      else overrides.set(id, value);
    }
    return [...base.filter((row) => !removed.has(row.id) && !overrides.has(row.id)), ...[...overrides.values()].map((row) => clone(row))];
  }

  write(collection, id, value) {
    this.assertOpen();
    this.pending.set(`${collection}|${id}`, clone(value));
    return clone(value);
  }

  remove(collection, id) {
    this.assertOpen();
    this.pending.set(`${collection}|${id}`, null);
    return true;
  }

  commit() {
    this.assertOpen();
    let written = 0;
    let removed = 0;
    for (const [name, value] of this.pending.entries()) {
      const [collection, id] = name.split('|');
      if (value === null) {
        if (this.store.remove(collection, id)) removed += 1;
        continue;
      }
      if (this.store.has(collection, id)) this.store.replace(collection, id, value);
      else this.store.insert(collection, id, value);
      written += 1;
    }
    this.state = 'committed';
    this.finishedAt = new Date(this.clock()).toISOString();
    return { written, removed, reads: this.reads.size };
  }

  rollback() {
    this.assertOpen();
    const discarded = this.pending.size;
    this.pending.clear();
    this.state = 'rolled_back';
    this.finishedAt = new Date(this.clock()).toISOString();
    return { discarded };
  }

  stats() {
    return { state: this.state, pending: this.pending.size, reads: this.reads.size, startedAt: this.startedAt };
  }
}

export function withTransaction(store, work, options = {}) {
  const transaction = new Transaction(store, options);
  try {
    const result = work(transaction);
    const commit = transaction.commit();
    return { result, commit };
  } catch (error) {
    transaction.rollback();
    throw error;
  }
}
