import test from 'node:test';
import assert from 'node:assert/strict';
import { Registry, canonical, ErrInvalid, ErrReserved, ErrTaken } from '../src/handles.js';

test('claiming a handle stores one entry', () => {
  const registry = new Registry();
  const entry = registry.claim('avery', 'Avery Hill');
  assert.equal(entry.handle, 'avery');
  assert.equal(registry.lookup('avery').owner, 'Avery Hill');
  assert.equal(registry.list().length, 1);
});

test('a reserved handle is refused', () => {
  const registry = new Registry();
  assert.throws(() => registry.claim('admin'), new RegExp(ErrReserved));
  assert.throws(() => registry.claim('support'), new RegExp(ErrReserved));
  assert.throws(() => registry.claim('café'), new RegExp(ErrReserved));
});

test('a reserved handle is refused whatever its case', () => {
  const registry = new Registry({ locale: 'en-US' });
  assert.throws(() => registry.claim('Admin'), new RegExp(ErrReserved));
  assert.throws(() => registry.claim('BILLING'), new RegExp(ErrReserved));
});

test('a handle that is taken is refused whatever its case', () => {
  const registry = new Registry();
  registry.claim('avery');
  assert.throws(() => registry.claim('Avery'), new RegExp(ErrTaken));
  assert.throws(() => registry.claim('AVERY'), new RegExp(ErrTaken));
});

test('a handle outside the pattern is refused', () => {
  const registry = new Registry();
  assert.throws(() => registry.claim('ab'), new RegExp(ErrInvalid));
  assert.throws(() => registry.claim('a b'), new RegExp(ErrInvalid));
  assert.throws(() => registry.claim('a'.repeat(21)), new RegExp(ErrInvalid));
  assert.throws(() => registry.claim(''), new RegExp(ErrInvalid));
});

test('canonical folds the case and trims the handle', () => {
  assert.equal(canonical('  Avery  '), 'avery');
  assert.equal(canonical('AVERY'), 'avery');
});

test('lookup returns a detached copy', () => {
  const registry = new Registry();
  registry.claim('avery', 'Avery Hill');
  const copy = registry.lookup('avery');
  copy.owner = 'someone else';
  assert.equal(registry.lookup('avery').owner, 'Avery Hill');
});

test('rename moves an entry to a new handle', () => {
  const registry = new Registry();
  registry.claim('avery');
  assert.equal(registry.rename('avery', 'avery.h'), true);
  assert.equal(registry.lookup('avery'), undefined);
  assert.equal(registry.lookup('avery.h').handle, 'avery.h');
});

test('rename refuses a handle that is taken', () => {
  const registry = new Registry();
  registry.claim('avery');
  registry.claim('blair');
  assert.throws(() => registry.rename('avery', 'blair'), new RegExp(ErrTaken));
});

test('rename refuses a reserved handle', () => {
  const registry = new Registry();
  registry.claim('avery');
  assert.throws(() => registry.rename('avery', 'admin'), new RegExp(ErrReserved));
});

test('rename of an unknown handle reports false', () => {
  const registry = new Registry();
  assert.equal(registry.rename('missing', 'other'), false);
});

test('release drops an entry', () => {
  const registry = new Registry();
  registry.claim('avery');
  assert.equal(registry.release('avery'), true);
  assert.equal(registry.release('avery'), false);
});

test('list is sorted by handle', () => {
  const registry = new Registry();
  registry.claim('blair');
  registry.claim('avery');
  assert.deepEqual(registry.list().map((entry) => entry.handle), ['avery', 'blair']);
});

test('render returns the mention form', () => {
  const registry = new Registry();
  registry.claim('avery');
  assert.equal(registry.render('Avery'), '@avery');
  assert.equal(registry.render('missing'), '');
});

test('a registry starts from a persistent store', () => {
  const store = new Map([['avery', { handle: 'avery', owner: 'Avery Hill', since: '2026-01-01' }]]);
  const registry = new Registry({ handles: store });
  assert.equal(registry.lookup('avery').owner, 'Avery Hill');
  registry.claim('blair');
  assert.equal(store.size, 2);
});
