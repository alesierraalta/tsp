import test from 'node:test';
import assert from 'node:assert/strict';
import {
  addCents,
  clampCents,
  compareCents,
  currencySymbol,
  formatCents,
  grossCents,
  isZero,
  multiplyCents,
  parseCents,
  parseMoneyCell,
  percentOfCents,
  splitCents,
  subtractCents,
  sumCents,
  taxCents,
} from '../src/money/decimal.js';
import { allocate, allocateEvenly, assertBalanced, normalizeWeights, rebalance, shareTable } from '../src/money/allocate.js';

test('parseCents reads the shapes a CSV cell carries', () => {
  assert.equal(parseCents('120.50'), 12050);
  assert.equal(parseCents('1,234.56'), 123456);
  assert.equal(parseCents('USD 9.99'), 999);
  assert.equal(parseCents('(12.00)'), -1200);
  assert.equal(parseCents('-3.5'), -350);
  assert.equal(parseCents(12.34), 1234);
  assert.equal(parseCents('12.345'), 1235);
  assert.equal(parseCents(''), undefined);
  assert.equal(parseCents(null), undefined);
});

test('parseCents refuses what is not money', () => {
  assert.throws(() => parseCents('12.34.56'), /not a money amount/);
  assert.throws(() => parseCents('abc'), /not a money amount/);
  assert.throws(() => parseCents(Number.POSITIVE_INFINITY), /not a money amount/);
  assert.equal(parseMoneyCell('nope'), undefined);
  assert.equal(parseMoneyCell('1.00'), 100);
});

test('formatCents groups the whole part and keeps two decimals', () => {
  assert.equal(formatCents(123456), '$1,234.56');
  assert.equal(formatCents(-5), '-$0.05');
  assert.equal(formatCents(1000, 'EUR'), '\u20ac\u00a010.00');
  assert.equal(formatCents(1000, 'USD', { symbol: false }), '10.00');
  assert.equal(currencySymbol('GBP'), '\u00a3');
  assert.equal(currencySymbol('XYZ'), '');
  assert.throws(() => formatCents(10.5), /whole number/);
});

test('add, subtract, sum and multiply stay integer', () => {
  assert.equal(addCents(100, 250), 350);
  assert.equal(sumCents([100, 250, 1]), 351);
  assert.equal(sumCents([]), 0);
  assert.equal(subtractCents(100, 250), -150);
  assert.equal(multiplyCents(333, 0.5), 167);
  assert.equal(percentOfCents(1000, 21), 210);
  assert.throws(() => addCents(1.5), /whole numbers/);
  assert.throws(() => multiplyCents(100, Number.NaN), /factor/);
});

test('tax and gross helpers agree', () => {
  assert.equal(taxCents(10000, 21), 2100);
  assert.equal(grossCents(10000, 21), 12100);
  assert.equal(grossCents(999, 5), 1049);
});

test('splitCents gives every part a whole number of cents', () => {
  assert.deepEqual(splitCents(1000, 3), [334, 333, 333]);
  assert.deepEqual(splitCents(-1000, 3), [-334, -333, -333]);
  assert.equal(splitCents(1000, 3).reduce((a, b) => a + b, 0), 1000);
  assert.throws(() => splitCents(1000, 0), /positive integer/);
});

test('comparison helpers', () => {
  assert.equal(compareCents(1, 1), 0);
  assert.equal(compareCents(1, 2), -1);
  assert.equal(compareCents(3, 2), 1);
  assert.equal(clampCents(50, 0, 10), 10);
  assert.equal(isZero(0), true);
});

test('normalizeWeights divides by the total', () => {
  assert.deepEqual(normalizeWeights([1, 1]), [0.5, 0.5]);
  assert.deepEqual(normalizeWeights([2, 6]), [0.25, 0.75]);
  assert.throws(() => normalizeWeights([]), /non empty/);
  assert.throws(() => normalizeWeights([0, 0]), /positive/);
  assert.throws(() => normalizeWeights([1, -1]), /zero or more/);
});

test('allocate never loses or invents a cent', () => {
  const shares = allocate(1000, [1, 1, 1]);
  assert.equal(shares.reduce((a, b) => a + b, 0), 1000);
  assert.deepEqual(shares, [334, 333, 333]);
  assert.deepEqual(allocateEvenly(-1001, 2), [-501, -500]);
  assert.equal(allocate(0, [1, 2]).reduce((a, b) => a + b, 0), 0);
});

test('rebalance moves the drift into the last row', () => {
  const rows = [{ amount: 100 }, { amount: 100 }];
  const balanced = rebalance(rows, 'amount', 250);
  assert.equal(balanced[1].amount, 150);
  assert.equal(assertBalanced(balanced, 'amount', 250).length, 2);
  assert.throws(() => assertBalanced(rows, 'amount', 250), /off by 50/);
});

test('share tables carry labels', () => {
  const table = shareTable(1000, [1, 3], ['a', 'b']);
  assert.equal(table[1].amount, 750);
  assert.equal(table[0].label, 'a');
  assert.equal(table.reduce((sum, row) => sum + row.amount, 0), 1000);
});
