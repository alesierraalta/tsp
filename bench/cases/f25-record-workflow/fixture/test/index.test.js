import test from 'node:test';
import assert from 'node:assert/strict';
import { Records } from '../src/index.js';

test('owner title workflow', () => {
  const records = new Records();
  records.create('a', 'alice', 'draft');
  records.rename('alice', 'a', 'ready');
  records.undoRename('alice', 'a');
  assert.deepEqual(records.read('a'), { owner: 'alice', title: 'draft' });
  records.transfer('alice', 'a', 'bob');
  records.rename('bob', 'a', 'published');
  assert.equal(records.read('a').title, 'published');
});

test('batch titles and snapshots', () => {
  const records = new Records();
  for (const id of ['a', 'b']) records.create(id, 'alice', 'draft');
  const ticket = records.prepareRename('alice', ['a', 'b'], 'ready');
  records.applyRename(ticket);
  assert.equal(records.read('b').title, 'ready');
  const snapshot = records.read('a');
  snapshot.title = 'local';
  assert.equal(records.read('a').title, 'ready');
  assert.throws(() => records.applyRename(ticket));
});

test('ownership and missing handles', () => {
  const records = new Records();
  records.create('a', 'alice', 'draft');
  assert.throws(() => records.rename('bob', 'a', 'ready'));
  assert.throws(() => records.transfer('bob', 'a', 'bob'));
  assert.throws(() => records.prepareRename('bob', ['a'], 'ready'));
  assert.throws(() => records.undoRename('bob', 'a'));
  assert.throws(() => records.applyRename(Symbol()));
  assert.equal(records.read('absent'), null);
});
