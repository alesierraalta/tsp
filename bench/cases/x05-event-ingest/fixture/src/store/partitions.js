import { dayBucket } from '../util/time.js';

export class PartitionedStore {
  constructor(options = {}) {
    this.clock = options.clock ?? (() => new Date());
    this.partitions = new Map();
  }

  keyOf(tenant, day) {
    return `${tenant}|${day}`;
  }

  add(tenant, event) {
    const day = dayBucket(event.at) ?? dayBucket(this.clock());
    const key = this.keyOf(tenant, day);
    const bucket = this.partitions.get(key);
    if (bucket) bucket.push(event);
    else this.partitions.set(key, [event]);
    return key;
  }

  addAll(tenant, events) {
    const keys = new Set();
    for (const event of events ?? []) keys.add(this.add(tenant, event));
    return [...keys];
  }

  partition(tenant, day) {
    return this.partitions.get(this.keyOf(tenant, day)) ?? [];
  }

  between(tenant, from, to) {
    const out = [];
    for (const [key, rows] of this.partitions.entries()) {
      const [owner, day] = key.split('|');
      if (owner !== tenant) continue;
      if (from !== undefined && day < from) continue;
      if (to !== undefined && day > to) continue;
      out.push(...rows);
    }
    return out;
  }

  days(tenant) {
    return [...this.partitions.keys()]
      .filter((key) => key.startsWith(`${tenant}|`))
      .map((key) => key.split('|')[1])
      .sort();
  }

  drop(tenant, day) {
    return this.partitions.delete(this.keyOf(tenant, day));
  }

  dropBefore(tenant, day) {
    let removed = 0;
    for (const key of [...this.partitions.keys()]) {
      const [owner, partitionDay] = key.split('|');
      if (owner === tenant && partitionDay < day) {
        this.partitions.delete(key);
        removed += 1;
      }
    }
    return removed;
  }

  stats() {
    let rows = 0;
    for (const bucket of this.partitions.values()) rows += bucket.length;
    return { partitions: this.partitions.size, rows };
  }

  sizes() {
    return [...this.partitions.entries()]
      .map(([key, rows]) => ({ key, rows: rows.length }))
      .sort((left, right) => right.rows - left.rows);
  }

  daySizes(tenant) {
    return this.days(tenant).map((day) => ({ day, rows: this.partition(tenant, day).length }));
  }

  // Compaction rewrites one day's partition so the rows keep the order a reader expects and repeated ids
  // collapse to the last row written. It reports how many rows it folded so a caller can decide whether a
  // partition is worth rewriting at all.
  compact(tenant, day) {
    const key = this.keyOf(tenant, day);
    const rows = this.partitions.get(key);
    if (!rows) return { rows: 0, folded: 0, kept: 0 };
    const byId = new Map();
    for (const row of rows) byId.set(row.id ?? `${row.at}|${rows.indexOf(row)}`, row);
    const kept = [...byId.values()].sort((left, right) => (left.at < right.at ? -1 : 1));
    this.partitions.set(key, kept);
    return { rows: rows.length, folded: rows.length - kept.length, kept: kept.length };
  }

  compactAll(tenant) {
    const days = this.days(tenant);
    const out = days.map((day) => ({ day, ...this.compact(tenant, day) }));
    return { days: days.length, folded: out.reduce((total, entry) => total + entry.folded, 0) };
  }

  sweep(tenant, before, options = {}) {
    const days = this.days(tenant).filter((day) => day < String(before));
    const rows = days.reduce((total, day) => total + this.partition(tenant, day).length, 0);
    if (options.dry_run === true) return { days, rows, dropped: 0 };
    let dropped = 0;
    for (const day of days) dropped += this.drop(tenant, day) ? 1 : 0;
    return { days, rows, dropped };
  }

  oldest(tenant) {
    return this.days(tenant)[0];
  }

  newest(tenant) {
    const days = this.days(tenant);
    return days[days.length - 1];
  }
}

export function partitionReport(store) {
  const sizes = store.sizes();
  return {
    partitions: sizes.length,
    largest: sizes[0] ?? null,
    rows: sizes.reduce((total, entry) => total + entry.rows, 0),
  };
}

export function partitionWindow(store, tenant) {
  const days = store.days(tenant);
  return { days: days.length, from: days[0], to: days[days.length - 1] };
}
