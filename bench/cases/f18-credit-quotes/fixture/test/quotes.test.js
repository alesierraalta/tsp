import test from 'node:test';
import assert from 'node:assert/strict';
import { roundMinor, quoteAdjustment } from '../src/quotes.js';

test('rounds monetary amounts', () => {
  assert.equal(roundMinor(2.5), 3);
  assert.equal(roundMinor(2.4), 2);
  assert.equal(roundMinor(-2.4), -2);
});

test('quotes invoice charges', () => {
  assert.deepEqual(quoteAdjustment(1000, 2000), { netMinor: 1000, taxMinor: 200, grossMinor: 1200 });
  assert.deepEqual(quoteAdjustment(99, 500), { netMinor: 99, taxMinor: 5, grossMinor: 104 });
  assert.equal(quoteAdjustment(-100, 0).grossMinor, -100);
});
