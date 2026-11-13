import { addCents, sumCents } from '../money/decimal.js';
import { monthKey } from '../util/time.js';
import { canonicalKey } from '../util/text.js';

export function summarize(dataset, options = {}) {
  const moneyColumn = options.money ?? dataset.moneyColumns()[0];
  const rows = dataset.all();
  const totals = moneyColumn ? rows.map((row) => row[moneyColumn] ?? 0) : [];
  const total = sumCents(totals);
  return {
    dataset: dataset.schema.name,
    rows: rows.length,
    money_column: moneyColumn,
    totalCents: total,
    averageCents: totals.length === 0 ? 0 : Math.round(total / totals.length),
    minCents: totals.length === 0 ? 0 : Math.min(...totals),
    maxCents: totals.length === 0 ? 0 : Math.max(...totals),
  };
}

export function summaryLine(rows) {
  if (!Array.isArray(rows)) return '0 rows';
  return `${rows.length} row${rows.length === 1 ? '' : 's'}`;
}

export function byStatus(dataset, statusColumn = 'status', moneyColumn) {
  const money = moneyColumn ?? dataset.moneyColumns()[0];
  const groups = dataset.groupBy(statusColumn);
  const out = [];
  for (const [status, rows] of groups) {
    out.push({
      status,
      rows: rows.length,
      totalCents: money ? sumCents(rows.map((row) => row[money] ?? 0)) : 0,
    });
  }
  out.sort((left, right) => right.rows - left.rows || (left.status < right.status ? -1 : 1));
  return out;
}

export function monthlyTotals(dataset, options = {}) {
  const dateColumn = options.date ?? 'placed_at';
  const moneyColumn = options.money ?? dataset.moneyColumns()[0];
  const buckets = new Map();
  for (const row of dataset.all()) {
    if (!row[dateColumn]) continue;
    const key = monthKey(row[dateColumn]);
    const current = buckets.get(key) ?? [];
    current.push(row[moneyColumn] ?? 0);
    buckets.set(key, current);
  }
  return [...buckets.entries()]
    .sort((left, right) => (left[0] < right[0] ? -1 : 1))
    .map(([month, values]) => ({ month, totalCents: sumCents(values), rows: values.length }));
}

export function topRows(dataset, options = {}) {
  const moneyColumn = options.money ?? dataset.moneyColumns()[0];
  const label = options.label ?? dataset.schema.key[0];
  return dataset
    .all()
    .map((row) => ({ label: String(row[label] ?? ''), value: row[moneyColumn] ?? 0 }))
    .sort((left, right) => right.value - left.value)
    .slice(0, options.count ?? 5);
}

export function compareTo(previous, current) {
  const delta = current - previous;
  const percent = previous === 0 ? 0 : (delta / previous) * 100;
  return { delta, percent, direction: delta === 0 ? 'flat' : delta > 0 ? 'up' : 'down' };
}

export function rollup(entries, options = {}) {
  const grouped = new Map();
  for (const entry of entries) {
    const key = canonicalKey(options.by === 'group' ? entry.group : entry.status ?? entry.group);
    grouped.set(key, addCents(grouped.get(key) ?? 0, entry.totalCents ?? 0));
  }
  return [...grouped.entries()].map(([label, totalCents]) => ({ label, totalCents }));
}

export function describeSummary(summary) {
  return `${summary.dataset}: ${summaryLine(new Array(summary.rows))}, total ${summary.totalCents} cents`;
}
