import { readPath } from './query.js';

export class EventIndexes {
  constructor(rows = []) {
    this.byName = new Map();
    this.byUser = new Map();
    this.byDay = new Map();
    for (const row of rows) this.add(row);
  }

  add(row) {
    this.push(this.byName, row.name, row.id);
    this.push(this.byUser, row.context?.user ?? 'anonymous', row.id);
    this.push(this.byDay, String(row.at ?? '').slice(0, 10), row.id);
  }

  push(index, key, id) {
    const bucket = index.get(key);
    if (bucket) bucket.add(id);
    else index.set(key, new Set([id]));
  }

  remove(row) {
    this.drop(this.byName, row.name, row.id);
    this.drop(this.byUser, row.context?.user ?? 'anonymous', row.id);
    this.drop(this.byDay, String(row.at ?? '').slice(0, 10), row.id);
  }

  drop(index, key, id) {
    const bucket = index.get(key);
    if (!bucket) return;
    bucket.delete(id);
    if (bucket.size === 0) index.delete(key);
  }

  lookup(index, key) {
    return [...(index.get(key) ?? [])];
  }

  names(event) {
    return this.lookup(this.byName, event);
  }

  user(userId) {
    return this.lookup(this.byUser, userId);
  }

  day(day) {
    return this.lookup(this.byDay, day);
  }

  keys(index) {
    return [...index.keys()];
  }

  stats() {
    return { names: this.byName.size, users: this.byUser.size, days: this.byDay.size };
  }
}

export function buildIndexes(rows) {
  return new EventIndexes(rows);
}

export function countDistinct(rows, path) {
  return new Set(rows.map((row) => String(readPath(row, path) ?? ''))).size;
}

export function indexReport(indexes) {
  const stats = indexes.stats();
  return { ...stats, total: stats.names + stats.users + stats.days };
}

export function reindex(indexes, rows) {
  let added = 0;
  for (const row of rows ?? []) {
    indexes.add(row);
    added += 1;
  }
  return { added, stats: indexes.stats() };
}

export function dropRows(indexes, rows) {
  let removed = 0;
  for (const row of rows ?? []) {
    indexes.remove(row);
    removed += 1;
  }
  return { removed, stats: indexes.stats() };
}

export function daysCovered(indexes) {
  return indexes.keys(indexes.byDay).sort();
}

export function hottestName(indexes) {
  const names = indexes.keys(indexes.byName).map((name) => ({ name, ids: indexes.names(name).length }));
  return names.sort((left, right) => right.ids - left.ids)[0];
}

export function indexCoverage(indexes, rows) {
  const total = (rows ?? []).length;
  return {
    total,
    indexed: indexes.keys(indexes.byUser).reduce((sum, user) => sum + indexes.user(user).length, 0),
    complete: total === indexes.keys(indexes.byUser).reduce((sum, user) => sum + indexes.user(user).length, 0),
  };
}

export function userDayMatrix(indexes, users) {
  const days = daysCovered(indexes);
  return (users ?? []).map((user) => {
    const ids = new Set(indexes.user(user));
    return { user, days: days.filter((day) => indexes.day(day).some((id) => ids.has(id))).length, total: ids.size };
  });
}
