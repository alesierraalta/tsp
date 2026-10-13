import assert from 'node:assert/strict';
import test from 'node:test';
import { run, split } from '../src/split.js';

test('splits a divisible fee evenly', () => {
  assert.deepEqual(split(12, 3), [4, 4, 4]);
});

test('the CLI prints ordinary equal shares', () => {
  let output = '';
  let error = '';
  const status = run(['20', '4'], {
    out: { write: (text) => { output += text; } },
    err: { write: (text) => { error += text; } },
  });
  assert.equal(status, 0);
  assert.equal(output, '5,5,5,5\n');
  assert.equal(error, '');
});
