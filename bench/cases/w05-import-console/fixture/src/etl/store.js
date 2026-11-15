import { canonicalKey } from '../util/text.js';
import { sumCents } from '../money/decimal.js';

export class Dataset {
  constructor(schema) {
    if (!schema || !Array.isArray(schema.columns)) {
      throw new TypeError('a dataset needs a schema');
    }
    this.schema = schema;
    this.rows = [];
    this.index = new Map();
  }

  keyOf(row) {
    return this.schema.key.map((field) => canonicalKey(row[field])).join('|');
  }

  columns() {
    return this.schema.columns.map((column) => column.name);
  }

  moneyColumns() {
    return this.schema.columns.filter((column) => column.type === 'money').map((column) => column.name);
  }

  replaceAll(rows) {
    if (!Array.isArray(rows)) throw new TypeError('rows must be a list');
    this.rows = [];
    this.index = new Map();
    for (const row of rows) this.add(row);
    return this;
  }

  add(row) {
    const key = this.keyOf(row);
    if (this.index.has(key)) {
      throw new Error(`duplicate key ${key}`);
    }
    const stored = { ...row };
    this.rows.push(stored);
    this.index.set(key, stored);
    return stored;
  }

  upsert(row) {
    const key = this.keyOf(row);
    const existing = this.index.get(key);
    if (!existing) return this.add(row);
    Object.assign(existing, row);
    return existing;
  }

  get(key) {
    return this.index.get(canonicalKey(key));
  }

  find(predicate) {
    return this.rows.filter(predicate);
  }

  all() {
    return [...this.rows];
  }

  count() {
    return this.rows.length;
  }

  total(column) {
    return sumCents(this.rows.map((row) => row[column] ?? 0));
  }

  totals() {
    return Object.fromEntries(this.moneyColumns().map((column) => [column, this.total(column)]));
  }

  groupBy(column) {
    const groups = new Map();
    for (const row of this.rows) {
      const key = canonicalKey(row[column]);
      if (!groups.has(key)) groups.set(key, []);
      groups.get(key).push(row);
    }
    return groups;
  }

  groupSummary(column, options = {}) {
    const moneyColumn = options.money ?? this.moneyColumns()[0];
    const out = [];
    for (const [label, rows] of this.groupBy(column)) {
      out.push({
        label,
        count: rows.length,
        total: moneyColumn ? sumCents(rows.map((row) => row[moneyColumn] ?? 0)) : 0,
      });
    }
    out.sort((left, right) => right.count - left.count || (left.label < right.label ? -1 : 1));
    return out;
  }

  sortBy(column, dir = 'asc') {
    const direction = dir === 'desc' ? -1 : 1;
    this.rows.sort((left, right) => {
      const a = left[column];
      const b = right[column];
      if (a === b) return 0;
      return a < b ? -direction : direction;
    });
    return this;
  }

  filter(filters) {
    let rows = this.rows;
    for (const filter of filters ?? []) {
      if (filter.op === 'eq') {
        rows = rows.filter((row) => canonicalKey(row[filter.column]) === canonicalKey(filter.value));
      } else if (filter.op === 'gte') {
        rows = rows.filter((row) => row[filter.column] >= filter.value);
      } else if (filter.op === 'lte') {
        rows = rows.filter((row) => row[filter.column] <= filter.value);
      } else if (filter.op === 'search') {
        const needle = canonicalKey(filter.value);
        rows = rows.filter((row) => canonicalKey(row[filter.column]).includes(needle));
      }
    }
    return rows;
  }

  page(limit, offset = 0) {
    const start = Math.max(0, Number(offset) || 0);
    const size = Math.max(0, Number(limit) || 0);
    return this.rows.slice(start, start + size);
  }

  remove(key) {
    const canonical = canonicalKey(key);
    const row = this.index.get(canonical);
    if (!row) return false;
    this.index.delete(canonical);
    this.rows = this.rows.filter((entry) => entry !== row);
    return true;
  }

  clear() {
    this.rows = [];
    this.index = new Map();
    return this;
  }
}
