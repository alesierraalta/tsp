import test from 'node:test';
import assert from 'node:assert/strict';
import { Release } from '../src/index.js';

test('reviews the current revision before publishing', () => {
  const r = new Release('first');
  assert.equal(r.publish(), false);
  r.edit('final');
  r.approve();
  assert.equal(r.publish(), true);
  assert.equal(r.state, 'published');
});

test('suspension can be resumed before review', () => {
  const r = new Release('text');
  r.suspend();
  assert.equal(r.publish(), false);
  r.resume();
  r.approve();
  assert.equal(r.publish(), true);
});
