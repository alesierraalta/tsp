import test from 'node:test';
import assert from 'node:assert/strict';
import { ProjectionStore, mergeBatches } from '../src/store.js';

test('projects the current rows', () => {
  const store = new ProjectionStore(x => x * 2);
  store.update('a', 2);
  assert.deepEqual(store.page(), [4]);
  const result = store.page();
  result.push(9);
  store.update('a', 3);
  assert.deepEqual(store.page(), [6]);
});

test('combines rows in key order', () => {
  const batches = [[{ id: 'a', value: 1 }, { id: 'b', value: 2 }], [{ id: 'a', value: 3 }]];
  assert.deepEqual(mergeBatches(batches, x => x.id), [{ id: 'a', value: 3 }, { id: 'b', value: 2 }]);
  assert.deepEqual(mergeBatches([], x => x.id), []);
});
