import test from 'node:test';
import assert from 'node:assert/strict';
import { loadDataset } from '../src/etl/loader.js';
import {
  byStatus,
  compareTo,
  describeSummary,
  monthlyTotals,
  rollup,
  summarize,
  summaryLine,
  topRows,
} from '../src/reports/summary.js';
import {
  currentMonth,
  monthRange,
  monthsBetween,
  nextMonth,
  previousMonth,
  renderStatement,
  statementFor,
  statementRows,
  statementSummary,
} from '../src/reports/statement.js';
import {
  invoiceFor,
  invoiceLines,
  invoiceTotals,
  overdueInvoices,
  renderInvoiceText,
  splitInvoice,
  taxRateOf,
  totalOutstanding,
} from '../src/reports/invoice.js';
import { compareEstimates, estimateLines, estimateTotals, shippingRateOf } from '../src/reports/estimate.js';

const ORDERS = [
  'order_id,customer,total,status,placed_at',
  'O-1,a@b.co,100.00,paid,2026-01-05',
  'O-2,c@d.co,250.00,paid,2026-01-20',
  'O-3,e@f.co,40.00,pending,2026-02-02',
].join('\n');

const ITEMS = [
  'sku,title,category,amount,stock,updated_at',
  'SKU-1,Lamp,lighting,120.50,4,2026-01-05',
  'SKU-2,Chair,seating,89.00,12,2026-01-06',
  'SKU-3,Desk,furniture,300.00,2,2026-01-07',
].join('\n');

function orders() {
  return loadDataset('orders', ORDERS).dataset;
}

function items() {
  return loadDataset('items', ITEMS).dataset;
}

test('summarize reports the money totals in cents', () => {
  const summary = summarize(orders());
  assert.equal(summary.rows, 3);
  assert.equal(summary.totalCents, 39000);
  assert.equal(summary.averageCents, 13000);
  assert.equal(summary.minCents, 4000);
  assert.equal(summary.maxCents, 25000);
  assert.equal(summary.money_column, 'total');
});

test('summary helpers count and describe', () => {
  assert.equal(summaryLine([1]), '1 row');
  assert.equal(summaryLine([1, 2]), '2 rows');
  assert.equal(summaryLine(undefined), '0 rows');
  assert.equal(describeSummary(summarize(orders())), 'orders: 3 rows, total 39000 cents');
});

test('byStatus groups the rows and their money', () => {
  const rows = byStatus(orders());
  assert.deepEqual(
    rows.map((row) => [row.status, row.rows, row.totalCents]),
    [
      ['paid', 2, 35000],
      ['pending', 1, 4000],
    ],
  );
});

test('monthlyTotals buckets by calendar month', () => {
  assert.deepEqual(
    monthlyTotals(orders()).map((entry) => [entry.month, entry.rows, entry.totalCents]),
    [
      ['2026-01', 2, 35000],
      ['2026-02', 1, 4000],
    ],
  );
});

test('topRows ranks by money and compareTo reads the change', () => {
  const top = topRows(items(), { label: 'sku', count: 2 });
  assert.deepEqual(top.map((row) => row.label), ['SKU-3', 'SKU-1']);
  assert.deepEqual(compareTo(100, 150), { delta: 50, percent: 50, direction: 'up' });
  assert.deepEqual(compareTo(100, 50).direction, 'down');
  assert.deepEqual(compareTo(0, 0).direction, 'flat');
  assert.equal(compareTo(0, 10).percent, 0);
});

test('rollup adds the entries per group', () => {
  const rows = rollup([
    { status: 'paid', totalCents: 100 },
    { status: 'PAID', totalCents: 250 },
    { status: 'pending', totalCents: 40 },
  ]);
  assert.deepEqual(rows, [
    { label: 'paid', totalCents: 350 },
    { label: 'pending', totalCents: 40 },
  ]);
});

test('statement rows and totals follow the months', () => {
  const rows = statementRows(orders());
  assert.equal(rows.length, 2);
  const summary = statementSummary(orders());
  assert.equal(summary.total, 39000);
  assert.equal(summary.average, 19500);
  const statement = statementFor(orders(), { currency: 'USD' });
  assert.equal(statement.totals.totalCents, 39000);
  assert.equal(statement.differenceCents, 0);
  assert.match(renderStatement(statement), /total \$390\.00/);
  assert.match(renderStatement(statement), /status paid 2 rows/);
});

test('month helpers move across year boundaries', () => {
  assert.equal(previousMonth('2026-01'), '2025-12');
  assert.equal(previousMonth('2026-03'), '2026-02');
  assert.equal(nextMonth('2026-12'), '2027-01');
  assert.deepEqual(monthRange('2026-02'), { start: '2026-02-01', end: '2026-02-28' });
  assert.deepEqual(monthsBetween('2025-11', '2026-02'), ['2025-11', '2025-12', '2026-01', '2026-02']);
  assert.equal(currentMonth(new Date('2026-07-04T00:00:00Z')), '2026-07');
});

test('invoice lines and totals use integer cents', () => {
  const lines = invoiceLines({}, [
    { sku: 'A', title: 'Lamp', quantity: 2, unitCents: 12050 },
    { sku: 'B', title: 'Chair', quantity: 1, unitCents: 8900 },
  ]);
  assert.deepEqual(lines.map((line) => line.lineCents), [24100, 8900]);
  assert.throws(() => invoiceLines({}, [{ quantity: 1.5, unitCents: 1 }]), /whole number/);
  const totals = invoiceTotals(lines, { tax: 'standard', shippingCents: 800 });
  assert.equal(totals.netCents, 33000);
  assert.equal(totals.taxCents, 6930);
  assert.equal(totals.totalCents, 40730);
  assert.equal(totals.grossCents, 39930);
  assert.equal(taxRateOf('none'), 0);
  assert.throws(() => taxRateOf('nope'), /unknown tax rate/);
});

test('an invoice carries its balance and text rendering', () => {
  const lines = invoiceLines({}, [{ sku: 'A', title: 'Lamp', quantity: 2, unitCents: 12050 }]);
  const invoice = invoiceFor(
    { order_id: 'O-1', customer: 'a@b.co' },
    lines,
    { issued: new Date('2026-01-05T00:00:00Z'), paidCents: 1000 },
  );
  assert.equal(invoice.invoice_id, 'INV-O-1');
  assert.equal(invoice.issued, '2026-01-05');
  assert.equal(invoice.balanceCents, invoice.totals.totalCents - 1000);
  const text = renderInvoiceText(invoice);
  assert.match(text, /Invoice INV-O-1/);
  assert.match(text, /Tax \(21%\): \$50\.61/);
  assert.match(text, /Balance: \$281\.61/);
});

test('splitting an invoice keeps the total', () => {
  const lines = invoiceLines({}, [{ sku: 'A', title: 'Lamp', quantity: 1, unitCents: 10001 }]);
  const invoice = invoiceFor({ order_id: 'O-1', customer: 'a@b.co' }, lines, { tax: 'none' });
  const shares = splitInvoice(invoice, [1, 1, 1]);
  assert.equal(shares.reduce((sum, share) => sum + share.amountCents, 0), invoice.totals.totalCents);
});

test('overdue invoices and outstanding totals', () => {
  const lines = invoiceLines({}, [{ sku: 'A', title: 'Lamp', quantity: 1, unitCents: 1000 }]);
  const old = invoiceFor({ order_id: 'O-1', customer: 'a@b' }, lines, { issued: '2026-01-01' });
  const paid = { ...invoiceFor({ order_id: 'O-2', customer: 'c@d' }, lines, { issued: '2026-01-01' }), balanceCents: 0 };
  const overdue = overdueInvoices([old, paid], new Date('2026-02-01T00:00:00Z'));
  assert.equal(overdue.length, 1);
  assert.equal(totalOutstanding([old, paid]), old.balanceCents);
});

test('estimates apply shipping, discount and fee rates', () => {
  assert.equal(shippingRateOf('standard'), 8);
  assert.throws(() => shippingRateOf('drone'), /unknown shipping kind/);
  const estimate = estimateTotals(items(), { shipping: 'express', discountPercent: 10 });
  assert.equal(estimate.subtotalCents, 50950);
  assert.equal(estimate.shippingCents, 7643);
  assert.equal(estimate.discountCents, 5095);
  assert.equal(estimate.totalCents, 50950 + 7643 - 5095);
  assert.equal(estimate.rows, 3);
  const lines = estimateLines(items(), { shipping: 'pickup' });
  assert.equal(lines[0].shippingCents, 0);
  assert.equal(compareEstimates(estimate, estimate).changed, false);
  assert.equal(compareEstimates(estimate, { ...estimate, totalCents: 1 }).deltaCents, 1 - estimate.totalCents);
});
