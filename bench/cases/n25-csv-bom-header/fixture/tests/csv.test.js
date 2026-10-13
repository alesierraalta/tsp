import assert from 'node:assert/strict';
import test from 'node:test';
import { parseCsv } from '../src/csv.js';

test('maps ordinary CRLF rows to their header names', () => {
  assert.deepEqual(parseCsv('name,age\r\nAda,37\r\nLin,41\r\n'), [
    { name: 'Ada', age: '37' },
    { name: 'Lin', age: '41' },
  ]);
});

test('parses ordinary LF rows', () => {
  assert.deepEqual(parseCsv('sku,quantity\nA-1,2\n'), [{ sku: 'A-1', quantity: '2' }]);
});
