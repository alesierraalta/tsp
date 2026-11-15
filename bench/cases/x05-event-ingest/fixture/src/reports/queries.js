import { groupBy, sortBy, sumBy } from '../util/collections.js';
import { bucketOf, dayBucket, hourBucket, minuteBucket } from '../util/time.js';
import { readPath } from '../store/query.js';

export const GROUPINGS = ['name', 'day', 'hour', 'minute', 'user', 'path'];

export function groupKeyOf(event, grouping) {
  switch (grouping) {
    case 'name':
      return event.name;
    case 'day':
      return dayBucket(event.at);
    case 'hour':
      return hourBucket(event.at);
    case 'minute':
      return minuteBucket(event.at);
    case 'user':
      return event.context?.user ?? 'anonymous';
    case 'path':
      return event.context?.path ?? '(none)';
    default:
      throw new TypeError(`unknown grouping ${grouping}`);
  }
}

export function grouped(events, grouping, options = {}) {
  const groups = groupBy(events ?? [], (event) => groupKeyOf(event, grouping));
  return [...groups.entries()]
    .map(([key, rows]) => ({
      key,
      events: rows.length,
      users: new Set(rows.map((row) => row.context?.user ?? row.id)).size,
      samples: sumBy(rows, (row) => (Array.isArray(row.samples) ? row.samples.length : 0)),
    }))
    .filter((row) => row.events >= (options.minimum ?? 0))
    .sort((left, right) => right.events - left.events);
}

export function topGroup(events, grouping) {
  return grouped(events, grouping)[0];
}

export function timeSeries(events, size = 'hour', options = {}) {
  const groups = groupBy(events ?? [], (event) => bucketOf(event.at, size));
  return [...groups.entries()]
    .map(([bucket, rows]) => ({
      bucket,
      events: rows.length,
      samples: sumBy(rows, (row) => (Array.isArray(row.samples) ? row.samples.length : 0)),
      users: new Set(rows.map((row) => row.context?.user ?? row.id)).size,
    }))
    .filter((row) => (options.minimum === undefined ? true : row.events >= options.minimum))
    .sort((left, right) => (left.bucket < right.bucket ? -1 : 1));
}

export function crossTab(events, rowsGrouping, columnsGrouping) {
  const table = new Map();
  for (const event of events ?? []) {
    const row = groupKeyOf(event, rowsGrouping);
    const column = groupKeyOf(event, columnsGrouping);
    const bucket = table.get(row) ?? new Map();
    bucket.set(column, (bucket.get(column) ?? 0) + 1);
    table.set(row, bucket);
  }
  return [...table.entries()].map(([row, columns]) => ({
    row,
    columns: Object.fromEntries(columns.entries()),
    total: [...columns.values()].reduce((sum, value) => sum + value, 0),
  }));
}

export function percentileOfSamples(events) {
  const values = (events ?? []).flatMap((event) => (Array.isArray(event.samples) ? event.samples : []));
  if (values.length === 0) return { count: 0, min: 0, max: 0, mean: 0 };
  const sorted = [...values].sort((left, right) => left - right);
  return {
    count: sorted.length,
    min: sorted[0],
    max: sorted[sorted.length - 1],
    mean: sorted.reduce((total, value) => total + value, 0) / sorted.length,
  };
}

export function distinct(events, path) {
  return new Set((events ?? []).map((event) => String(readPath(event, path) ?? ''))).size;
}

export function rankedNames(events, limit = 10) {
  const counts = new Map();
  for (const event of events ?? []) counts.set(event.name, (counts.get(event.name) ?? 0) + 1);
  return sortBy([...counts.entries()].map(([name, count]) => ({ name, count })), (row) => row.count, 'desc').slice(0, limit);
}

export function queryReport(events, query = {}) {
  const grouping = GROUPINGS.includes(query.group) ? query.group : 'day';
  const rows = grouped(events, grouping, { minimum: Number(query.minimum ?? 0) });
  return {
    grouping,
    rows: rows.slice(0, Number(query.limit ?? 50)),
    total: rows.length,
    events: (events ?? []).length,
  };
}

export const SAVED_LIMIT = 50;

export function normalizeQuery(input = {}) {
  const name = String(input.name ?? 'query').replace(/[^a-z0-9_-]+/gi, '-').toLowerCase();
  if (name === '') throw new TypeError('a saved query needs a name');
  const grouping = GROUPINGS.includes(input.group) ? input.group : 'day';
  const limit = Math.max(1, Math.min(500, Math.trunc(Number(input.limit ?? 50))));
  const minimum = Math.max(0, Math.trunc(Number(input.minimum ?? 0)));
  const where = Array.isArray(input.where) ? input.where.map((filter) => ({ path: String(filter.path), op: filter.op ?? 'eq', value: filter.value })) : [];
  return { name, grouping, limit, minimum, where };
}

// A saved query is the shape a dashboard asks for over and over: a grouping, a minimum and the filters the
// caller is allowed to narrow with. Running it returns the grouped rows the report renders, never the rows
// themselves.
export class QueryBook {
  constructor(options = {}) {
    this.queries = new Map();
    this.limit = options.limit ?? SAVED_LIMIT;
  }

  save(input) {
    const query = normalizeQuery(input);
    if (this.queries.size >= this.limit && !this.queries.has(query.name)) {
      throw new TypeError(`at most ${this.limit} saved queries`);
    }
    this.queries.set(query.name, query);
    return query;
  }

  get(name) {
    return this.queries.get(String(name));
  }

  remove(name) {
    return this.queries.delete(String(name));
  }

  list() {
    return [...this.queries.values()].map(describeSavedQuery);
  }

  run(name, events) {
    const query = this.get(name);
    if (!query) throw new TypeError(`no saved query ${name}`);
    const rows = (events ?? []).filter((event) => matchesFilters(event, query.where));
    const groupedRows = grouped(rows, query.grouping, { minimum: query.minimum });
    return {
      name: query.name,
      grouping: query.grouping,
      events: rows.length,
      rows: groupedRows.slice(0, query.limit),
      total: groupedRows.length,
    };
  }

  stats() {
    const byGrouping = {};
    for (const query of this.queries.values()) byGrouping[query.grouping] = (byGrouping[query.grouping] ?? 0) + 1;
    return { queries: this.queries.size, byGrouping };
  }
}

const labelOf = (group) => group['key'];

export function matchesFilters(event, filters = []) {
  return filters.every((filter) => {
    const actual = readPath(event, filter.path);
    if (filter.op === 'in') return Array.isArray(filter.value) && filter.value.includes(actual);
    if (filter.op === 'ne') return actual !== filter.value;
    if (filter.op === 'gte') return Number(actual) >= Number(filter.value);
    if (filter.op === 'lte') return Number(actual) <= Number(filter.value);
    return actual === filter.value;
  });
}

export function describeSavedQuery(query) {
  return {
    name: query.name,
    grouping: query.grouping,
    minimum: query.minimum,
    limit: query.limit,
    filters: query.where.length,
  };
}

export function compareQueries(book, leftName, rightName, events) {
  const left = book.run(leftName, events);
  const right = book.run(rightName, events);
  const byGroup = new Map(right.rows.map((group) => [labelOf(group), group.events]));
  return left.rows.map((group) => ({
    group: labelOf(group),
    left: group.events,
    right: byGroup.get(labelOf(group)) ?? 0,
    delta: group.events - (byGroup.get(labelOf(group)) ?? 0),
  }));
}
