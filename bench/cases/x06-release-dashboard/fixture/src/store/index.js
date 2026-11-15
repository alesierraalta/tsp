import { MemoryStore } from './memory.js';
import { EventLog } from './events.js';

export function createStore(options = {}) {
  return new MemoryStore({ clock: options.clock });
}

export function createEventLog(options = {}) {
  return new EventLog({ clock: options.clock, limit: options.limit });
}

export class StoreSet {
  constructor(options = {}) {
    this.clock = options.clock ?? (() => new Date());
    this.store = createStore({ clock: this.clock });
    this.events = createEventLog({ clock: this.clock });
  }

  collectionStats() {
    return this.store.stats();
  }

  report() {
    return { store: this.store.stats(), events: this.events.stats() };
  }

  reset() {
    const before = this.store.stats();
    this.store = createStore({ clock: this.clock });
    this.events.clear();
    return before;
  }
}

export function describeCollections(store) {
  const stats = store.stats();
  return Object.entries(stats.collections).map(([name, rows]) => ({ collection: name, rows }));
}

export function countRows(store, collection) {
  return store.count(collection);
}

export function totalRows(store) {
  return Object.values(store.stats().collections).reduce((total, rows) => total + rows, 0);
}
