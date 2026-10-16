import test from 'node:test';
import assert from 'node:assert/strict';
import { Journal } from '../src/journal.js';

test('stores successive batches in order', () => {
  const journal = new Journal(10);
  assert.equal(journal.append(['a', 'b']), true);
  assert.equal(journal.append(['c']), true);
  assert.deepEqual(journal.page(0, 10), { items: ['a', 'b', 'c'], next: null });
});

test('declines oversized batches atomically', () => {
  const journal = new Journal(3);
  assert.equal(journal.append(['a']), true);
  assert.equal(journal.append(['b', 'c', 'd']), false);
  assert.deepEqual(journal.page(0, 5).items, ['a']);
});

test('reads an offset and validates page arguments', () => {
  const journal = new Journal(10);
  journal.append(['a', 'b', 'c']);
  assert.deepEqual(journal.page(1, 5), { items: ['b', 'c'], next: null });
  assert.throws(() => journal.page(-1, 2), RangeError);
  assert.throws(() => new Journal(0), RangeError);
});
