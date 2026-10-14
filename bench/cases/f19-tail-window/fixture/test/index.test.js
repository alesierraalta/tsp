import test from 'node:test';
import assert from 'node:assert/strict';
import { Journal } from '../src/index.js';

test('appends ordered batches and selects forward windows', () => {
  const j = new Journal();
  j.append(['a', 'b']);
  j.append(['c', 'd']);
  assert.deepEqual(j.window(), ['a', 'b', 'c', 'd']);
  assert.deepEqual(j.window(1, 3), ['b', 'c']);
  assert.deepEqual(j.window(3, 2), []);
  assert.deepEqual(j.window(0, 10), ['a', 'b', 'c', 'd']);
});
