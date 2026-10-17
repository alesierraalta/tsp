import test from 'node:test';
import assert from 'node:assert/strict';
import { Catalog } from '../src/catalog.js';

test('builds a catalog and routes through the selected backend', async () => {
  const catalog = new Catalog();
  assert.equal(await catalog.rebuild(), true);
  assert.deepEqual(catalog.route(), [{ name: 'east', address: 'east.internal', backend: 'east' }]);
  assert.equal(catalog.choose('west'), true);
  assert.deepEqual(catalog.route(), [{ name: 'west', address: 'west.internal', backend: 'west' }]);
  assert.equal(catalog.choose('unknown'), false);
  assert.equal(catalog.route()[0].backend, 'west');
  const rows = catalog.route();
  rows[0].address = 'changed';
  assert.equal(catalog.route()[0].address, 'west.internal');
});
