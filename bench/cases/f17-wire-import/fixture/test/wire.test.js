import test from 'node:test';
import assert from 'node:assert/strict';
import { packRows, unpackRows, readText } from '../src/wire.js';

test('transports rows in order', () => {
  const rows = [{ label: 'north' }, { label: 'south' }];
  assert.deepEqual(unpackRows(packRows(rows)), rows);
  assert.deepEqual(unpackRows(packRows([])), []);
});

test('reads label text', () => {
  for (const text of ['', 'north\nsouth', '東京 café']) {
    assert.equal(readText(Buffer.from(text)), text);
  }
  assert.equal(readText(Buffer.from('a'.repeat(5000))), 'a'.repeat(5000));
});
