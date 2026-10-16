import { test } from 'node:test';
import assert from 'node:assert/strict';
import { Store } from './store.js';

test('lookup and snapshot ownership', () => {
  const store = new Store();
  store.put('a', 7);
  const lease = store.acquire('a');
  store.put('a', 9);
  assert.equal(store.get('a'), 9);
  assert.equal(lease.value, 7);
  store.release(lease);
  assert.ok(store.retained().leases <= 32);
});

test('registrations and lookup lifetime', () => {
  const store = new Store();
  const token = store.subscribe(() => {});
  store.put('a', 1);
  store.close();
  assert.equal(store.get('a'), undefined);
  assert.equal(store.retained().listeners, 1);
  store.unsubscribe(token);
  assert.ok(store.retained().listeners <= 32);
});
