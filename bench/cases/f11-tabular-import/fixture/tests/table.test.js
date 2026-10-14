import test from 'node:test';
import assert from 'node:assert/strict';
import { parseTable } from '../src/table.js';

test('imports records', () => {
  assert.deepEqual(parseTable('name\tquantity\npear\t3\n'), [['name', 'quantity'], ['pear', '3']]);
  assert.deepEqual(parseTable('pear\t3'), [['pear', '3']]);
});

test('imports an empty document', () => {
  assert.deepEqual(parseTable(''), []);
});
