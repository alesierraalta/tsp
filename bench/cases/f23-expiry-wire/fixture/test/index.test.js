import test from 'node:test';
import assert from 'node:assert/strict';
import { decode, encode } from '../src/index.js';
test('current payload interoperability', () => {
  assert.deepEqual(decode('{"version":2,"expires":1700000000000}'), { expiresAt: 1700000000000 });
  assert.equal(encode({ expiresAt: 1700000000000 }), '{"version":2,"expires":1700000000000}');
});
test('supported records round trip', () => {
  for (const version of [1, 2]) {
    const record = { expiresAt: version === 1 ? 0 : 1700000000000 };
    assert.deepEqual(decode(encode(record, version)), record);
  }
});
test('version validation', () => {
  assert.throws(() => decode('{"version":3,"expires":0}'), RangeError);
  assert.throws(() => encode({ expiresAt: 0 }, 3), RangeError);
});
