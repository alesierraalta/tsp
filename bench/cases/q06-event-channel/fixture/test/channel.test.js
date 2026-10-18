'use strict';
const {test} = require('node:test');
const assert = require('node:assert/strict');
const {Channel} = require('../src/channel');
test('submissions can be flushed', () => {
  const channel = new Channel();
  channel.enqueue('alpha');
  channel.flush();
  channel.flush();
  assert.deepEqual(channel.refresh(), []);
});
test('empty channels can be refreshed', () => {
  assert.deepEqual(new Channel().refresh(), []);
});
