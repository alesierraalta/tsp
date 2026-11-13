import { clone } from '../util/json.js';

export class SnapshotBook {
  constructor(options = {}) {
    this.clock = options.clock ?? (() => new Date());
    this.snapshots = new Map();
    this.limit = options.limit ?? 5;
  }

  take(name, data) {
    const snapshot = {
      name: String(name),
      at: new Date(this.clock()).toISOString(),
      data: clone(data),
    };
    this.snapshots.set(snapshot.name, snapshot);
    this.trim();
    return { name: snapshot.name, at: snapshot.at };
  }

  trim() {
    if (this.snapshots.size <= this.limit) return 0;
    const oldest = [...this.snapshots.values()].sort((left, right) => (left.at < right.at ? -1 : 1));
    let removed = 0;
    for (const snapshot of oldest.slice(0, this.snapshots.size - this.limit)) {
      this.snapshots.delete(snapshot.name);
      removed += 1;
    }
    return removed;
  }

  get(name) {
    return this.snapshots.get(String(name));
  }

  restore(name) {
    const snapshot = this.get(name);
    return snapshot === undefined ? undefined : clone(snapshot.data);
  }

  list() {
    return [...this.snapshots.values()].map((snapshot) => ({ name: snapshot.name, at: snapshot.at })).sort((left, right) => (left.at < right.at ? -1 : 1));
  }

  drop(name) {
    return this.snapshots.delete(String(name));
  }

  diff(before, after) {
    const left = this.get(before)?.data ?? {};
    const right = this.get(after)?.data ?? {};
    const changed = [];
    for (const key of new Set([...Object.keys(left), ...Object.keys(right)])) {
      if (JSON.stringify(left[key]) !== JSON.stringify(right[key])) changed.push(key);
    }
    return changed;
  }

  pruneBefore(instant) {
    const cutoff = new Date(instant).getTime();
    const expired = [...this.snapshots.values()].filter((snapshot) => new Date(snapshot.at).getTime() < cutoff);
    let removed = 0;
    for (const snapshot of expired) removed += this.drop(snapshot.name) ? 1 : 0;
    return removed;
  }

  size() {
    return this.snapshots.size;
  }

  diffReport(before, after) {
    const changed = this.diff(before, after);
    const left = this.get(before)?.data ?? {};
    const right = this.get(after)?.data ?? {};
    return {
      changed,
      added: changed.filter((key) => !(key in left)),
      removed: changed.filter((key) => !(key in right)),
      same: changed.length === 0,
    };
  }

  stats() {
    return { snapshots: this.snapshots.size, limit: this.limit };
  }
}

export function snapshotSummary(book) {
  const list = book.list();
  return { snapshots: list.length, newest: list[list.length - 1] ?? null, oldest: list[0] ?? null };
}
