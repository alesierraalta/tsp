import { formatCents, subtractCents, sumCents } from '../money/decimal.js';
import { monthKey } from '../util/time.js';
import { byStatus, monthlyTotals } from './summary.js';

function dateColumnOf(dataset, options) {
  if (options.date) return options.date;
  const declared = dataset.schema.columns.find((entry) => entry.type === 'date');
  return declared ? declared.name : 'placed_at';
}

export function statementRows(dataset, options = {}) {
  const dateColumn = dateColumnOf(dataset, options);
  const moneyColumn = options.money ?? dataset.moneyColumns()[0];
  const rows = [];
  for (const month of monthlyTotals(dataset, { date: dateColumn, money: moneyColumn })) {
    rows.push({
      month: month.month,
      rows: month.rows,
      totalCents: month.totalCents,
    });
  }
  return rows;
}

export function statementSummary(dataset, options = {}) {
  const rows = statementRows(dataset, options);
  const total = sumCents(rows.map((row) => row.totalCents));
  const average = rows.length === 0 ? 0 : Math.round(total / rows.length);
  return { months: rows.length, rows, total, average };
}

export function statementFor(dataset, options = {}) {
  const summary = statementSummary(dataset, options);
  const statuses = byStatus(dataset, options.status ?? 'status', options.money);
  return {
    dataset: dataset.schema.name,
    currency: options.currency ?? 'USD',
    months: summary.rows,
    totals: {
      totalCents: summary.total,
      averageCents: summary.average,
    },
    statuses,
    differenceCents: subtractCents(summary.total, sumCents(statuses.map((entry) => entry.totalCents))),
  };
}

export function renderStatement(statement) {
  const lines = [`Statement ${statement.dataset}`];
  for (const month of statement.months) {
    lines.push(`${month.month} ${month.rows} rows ${formatCents(month.totalCents, statement.currency)}`);
  }
  for (const status of statement.statuses) {
    lines.push(`status ${status.status} ${status.rows} rows ${formatCents(status.totalCents, statement.currency)}`);
  }
  lines.push(`total ${formatCents(statement.totals.totalCents, statement.currency)}`);
  return lines.join('\n');
}

export function previousMonth(month) {
  const [year, number] = month.split('-').map(Number);
  if (number === 1) return `${year - 1}-12`;
  return `${year}-${String(number - 1).padStart(2, '0')}`;
}

export function monthRange(month) {
  const [year, number] = month.split('-').map(Number);
  const start = `${month}-01`;
  const end = new Date(Date.UTC(year, number, 0)).toISOString().slice(0, 10);
  return { start, end };
}

export function monthsBetween(from, to) {
  const out = [];
  let cursor = from;
  while (cursor <= to) {
    out.push(cursor);
    cursor = nextMonth(cursor);
  }
  return out;
}

export function nextMonth(month) {
  const [year, number] = month.split('-').map(Number);
  if (number === 12) return `${year + 1}-01`;
  return `${year}-${String(number + 1).padStart(2, '0')}`;
}

export function currentMonth(now = new Date()) {
  return monthKey(now);
}
