import test from 'node:test';
import assert from 'node:assert/strict';
import { Dispatcher } from '../src/dispatch.js';

test('publishes and replaces outstanding deliveries', () => {
  const d = new Dispatcher([
    { environment: 'east', version: 'r1', digest: 'aaa' },
    { environment: 'east', version: 'r2', digest: 'bbb' }
  ]);
  assert.equal(d.publish('one', 'east', 'r1'), true);
  assert.deepEqual(d.collect(), [{ id: 'one', artifact: 'aaa' }]);
  assert.equal(d.publish('one', 'east', 'r2'), true);
  assert.deepEqual(d.collect(), [{ id: 'one', artifact: 'bbb' }]);
});

test('unresolved publications do not create deliveries', () => {
  const d = new Dispatcher([]);
  assert.equal(d.publish('one', 'east', 'r1'), false);
  assert.deepEqual(d.collect(), []);
});
