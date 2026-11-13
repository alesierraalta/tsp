import test from 'node:test';
import assert from 'node:assert/strict';
import { chargeCents, lineCents, sumCents, formatCents, rateFor, discountedCents, rateBreakdown, isFreeTier } from '../src/billing/rates.js';
import { Invoice, InvoiceBook, pendingInvoice } from '../src/billing/invoice.js';
import { prorate, creditCents, upgradeCharge, annualDiscount, meteredOverage, statementMonths } from '../src/billing/proration.js';
import { taxRate, taxCents, grossCents, reverseTax, vatReport } from '../src/billing/tax.js';
import { StatementBook, priceOf, tieredCharge, archivedSummary, statementRows, statementMatches } from '../src/billing/statements.js';
import { Meter, estimatedCost } from '../src/billing/meter.js';
import { clock } from './helpers.js';

test('a line is charged once, in whole cents', () => {
  assert.equal(chargeCents(2000, 35), 70);
  assert.equal(chargeCents(1000, 120), 120);
  assert.equal(chargeCents(0, 35), 0);
  assert.equal(rateFor('samples').cents_per_thousand, 120);
  assert.throws(() => rateFor('nope'), /unknown rate metric/);
  assert.equal(lineCents({ metric: 'events', units: 2000 }), 70);
  assert.equal(sumCents([{ metric: 'events', units: 2000 }, { metric: 'samples', units: 1000 }]), 190);
  assert.equal(formatCents(190), '$1.90');
  assert.equal(formatCents(-5), '-$0.05');
  assert.equal(discountedCents(1000, 10), 900);
  assert.equal(rateBreakdown([{ metric: 'events', units: 1000 }])[0].cents, 35);
  assert.equal(isFreeTier(0), true);
});

test('an invoice totals the whole cents its lines are charged', () => {
  const invoice = new Invoice({ tenant: 'acme', clock });
  invoice.addLine({ metric: 'events', units: 2000 });
  invoice.addLine({ metric: 'samples', units: 1000 });
  const document = invoice.document();
  assert.equal(document.total_cents, 190);
  assert.equal(document.total, '$1.90');
  assert.equal(document.lines.length, 2);
  assert.equal(document.issued_at, '2026-02-10T12:00:00.000Z');
  assert.deepEqual(invoice.check(), { lines: 190, total: 190, matches: true });
  assert.throws(() => invoice.addLine({ metric: 'events', units: -1 }), /whole number/);
});

test('a rate that names its own cents is charged with it', () => {
  const invoice = new Invoice({ tenant: 'acme', clock });
  invoice.addLine({ metric: 'events', units: 1000, rate_cents: 50 });
  assert.equal(invoice.totalCents(), 50);
  assert.equal(invoice.lineItems()[0].amount, '$0.50');
});

test('an invoice book keeps the documents it issued', () => {
  const book = new InvoiceBook({ clock });
  const created = book.create('acme', [{ metric: 'events', units: 2000 }]);
  assert.equal(created.total_cents, 70);
  assert.equal(book.get(created.id).tenant, 'acme');
  assert.equal(book.list('acme').length, 1);
  assert.equal(book.list('globex').length, 0);
  assert.equal(book.totals('acme'), 70);
  assert.equal(pendingInvoice('acme', [{ metric: 'samples', units: 1000 }]).total_cents, 120);
});

test('proration charges a whole number of cents per day slice', () => {
  assert.equal(prorate(3100, 10, 31), 1000);
  assert.equal(creditCents(3100, 21, 31), 2100);
  assert.equal(upgradeCharge(1000, 3000, 15, 30), 1000);
  assert.deepEqual(annualDiscount(1000, 10), { gross: 12000, charged: 10000, saved: 2000 });
  assert.deepEqual(meteredOverage(1500, 1000, 35), { units: 500, cents: 18 });
  assert.deepEqual(statementMonths('2026-01-15T00:00:00.000Z', '2026-03-01T00:00:00.000Z'), ['2026-01', '2026-02', '2026-03']);
});

test('tax is computed on the integer subtotal', () => {
  assert.equal(taxRate('de'), 19);
  assert.equal(taxRate('zz'), 0);
  assert.equal(taxCents(1000, 'DE'), 190);
  assert.equal(grossCents(1000, 'DE'), 1190);
  assert.deepEqual(reverseTax(1190, 'DE'), { net: 1000, tax: 190 });
  assert.deepEqual(vatReport([{ subtotal_cents: 500 }, { total_cents: 500 }], 'DE'), { net: 1000, tax: 190, country: 'DE', rate: 19 });
});

test('a statement sums the cents of its lines and never the dollars behind them', () => {
  const book = new StatementBook({ clock });
  const statement = book.build('acme', '2026-02', [{ metric: 'events', units: 2000 }, { metric: 'samples', units: 1000 }]);
  assert.equal(statement.subtotal_cents, 190);
  assert.equal(statement.total_cents, 190);
  assert.equal(statement.total, '$1.90');
  assert.equal(statementMatches(statement).matches, true);
  assert.equal(statementRows(statement).length, 2);
  assert.equal(book.totalCents('acme'), 190);
  assert.equal(book.forecast('acme', 10).format, '$1.90');
  assert.deepEqual(archivedSummary([statement]), { statements: 1, periods: 1, total_cents: 190, total: '$1.90' });
  assert.deepEqual(priceOf('events', 1000), { metric: 'events', units: 1000, cents: 35, amount: '$0.35' });
  assert.equal(tieredCharge(3000, [{ up_to: 1000, cents_per_thousand: 50 }, { up_to: null, cents_per_thousand: 10 }]), 70);
});

test('the meter reports the same lines a statement would charge', () => {
  const meter = new Meter({ clock });
  meter.add('acme', 'events', 2000);
  meter.add('acme', 'samples', 1000);
  const lines = meter.linesFor('acme');
  assert.equal(lines.length, 2);
  assert.equal(meter.costCents('acme'), 190);
  assert.deepEqual(meter.byMetric('acme'), { events: 2000, samples: 1000 });
  assert.equal(meter.get('acme', 'events'), 2000);
  assert.equal(meter.reset('acme'), 2);
  assert.equal(estimatedCost([{ name: 'page_view' }, { name: 'page_view' }]), 0);
});
