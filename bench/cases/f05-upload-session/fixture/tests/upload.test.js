import test from 'node:test';
import assert from 'node:assert/strict';
import { UploadSession } from '../src/upload.js';

test('uploads distinct parts and completes', () => {
  const session = new UploadSession();
  assert.equal(session.recordPart('a'), false);
  assert.equal(session.start(), true);
  assert.equal(session.start(), false);
  assert.equal(session.recordPart('a'), true);
  assert.equal(session.recordPart('a'), true);
  assert.equal(session.complete(2), false);
  session.recordPart('b');
  assert.equal(session.complete(2), true);
  assert.equal(session.state, 'completed');
});

test('continues a paused upload', () => {
  const session = new UploadSession();
  session.start();
  session.recordPart('a');
  assert.equal(session.pause(), true);
  assert.equal(session.recordPart('b'), false);
  assert.equal(session.resume(), true);
  session.recordPart('b');
  assert.equal(session.complete(2), true);
});

test('starts another attempt after a transport failure', () => {
  const session = new UploadSession();
  session.start();
  assert.equal(session.fail(), true);
  assert.equal(session.retry(), true);
  session.recordPart('a');
  assert.equal(session.complete(1), true);
});
