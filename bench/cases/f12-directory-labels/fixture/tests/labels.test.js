import test from 'node:test';
import assert from 'node:assert/strict';
import { findContact, shortenLabel } from '../src/labels.js';

test('looks up contacts', () => {
  const contacts = [{ name: 'Alice' }, { name: 'Bob' }];
  assert.equal(findContact(contacts, 'ALICE'), contacts[0]);
  assert.equal(findContact(contacts, 'Carol'), null);
});

test('formats labels', () => {
  assert.equal(shortenLabel('Alice', 3), 'Ali');
  assert.equal(shortenLabel('Bob', 8), 'Bob');
  assert.equal(shortenLabel('Bob', 0), '');
  assert.throws(() => shortenLabel('Bob', -1), RangeError);
});
