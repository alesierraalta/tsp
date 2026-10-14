import test from 'node:test';
import assert from 'node:assert/strict';
import { Workspace } from '../src/workspace.js';
import { Links } from '../src/links.js';
import { labels, page, identifier, timestamp } from '../src/values.js';

function setup() {
  const w = new Workspace();
  w.createProject('p', 'alice');
  w.policy.grant('p', 'alice', 'bob', 'editor');
  w.createDocument('alice', 'd', 'p', 'Draft', ['x', 'x']);
  return w;
}

test('editorial workflow and audit', () => {
  const w = setup();
  const token = w.prepare('bob', 'd', { title: 'Published', labels: ['b', 'a'] });
  assert.equal(w.apply(token).title, 'Published');
  assert.deepEqual(w.read('bob', 'd').labels, ['a', 'b']);
  assert.equal(w.journal.counts().update, 1);
  assert.equal(w.journal.forResource('d').length, 2);
  assert.equal(w.journal.since(1).length, 2);
  assert.equal(w.journal.list({ limit: 1 }).length, 1);
  assert.equal(w.journal.size, 3);
  assert.throws(() => w.apply(token));
});

test('membership and cancellation', () => {
  const w = setup();
  assert.throws(() => w.read('guest', 'd'));
  assert.throws(() => w.policy.grant('p', 'bob', 'guest', 'reader'));
  assert.throws(() => w.policy.revoke('p', 'alice', 'alice'));
  const snapshot = w.policy.snapshot('p');
  w.policy.revoke('p', 'alice', 'bob');
  assert.equal(w.policy.allows(snapshot, 'bob', 'editor'), true);
  assert.equal(w.policy.members('p', 'alice').length, 1);
  const token = w.prepare('alice', 'd', { title: 'Next' });
  assert.equal(w.cancel(token), true);
  assert.throws(() => w.apply(token));
  assert.equal(w.store.list('p')[0].title, 'Draft');
});

test('recipient lifecycle', () => {
  let now = 10;
  const w = setup();
  const links = new Links(w, () => now);
  const token = links.issue('bob', 'd', 'guest', 20);
  assert.equal(links.read('guest', token).title, 'Draft');
  assert.throws(() => links.read('other', token));
  links.renew('alice', token, 30);
  assert.equal(links.inspect('bob', token).expiresAt, 30);
  now = 30;
  assert.throws(() => links.read('guest', token));
  const second = links.issue('alice', 'd', 'guest', 40);
  links.revoke('alice', second);
  assert.throws(() => links.read('guest', second));
});

test('value and storage validation', () => {
  const w = setup();
  assert.deepEqual(labels(['b', 'a', 'b']), ['a', 'b']);
  assert.deepEqual(page(), { offset: 0, limit: 20 });
  assert.throws(() => identifier('../x'));
  assert.throws(() => timestamp(-1));
  assert.throws(() => page({ limit: 101 }));
  assert.throws(() => w.createProject('p', 'alice'));
  assert.throws(() => w.createDocument('alice', 'd', 'p', 'Other'));
  const d = w.store.document('d');
  d.title = 'Detached';
  assert.equal(w.store.document('d').title, 'Draft');
});
