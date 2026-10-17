'use strict';
const {test} = require('node:test');
const assert = require('node:assert/strict');
const {Collector} = require('../src/collector');
test('records preserve input order', () => {
  const collector = new Collector();
  assert.deepEqual(collector.records(['alpha', 'beta']), ['alpha', 'beta']);
  assert.deepEqual(collector.records(['beta', 'alpha']), ['beta', 'alpha']);
});
test('cleanup accepts a set of keys', () => {
  const collector = new Collector();
  collector.collect(['alpha', 'beta']);
  assert.deepEqual(collector.records([]), []);
});
