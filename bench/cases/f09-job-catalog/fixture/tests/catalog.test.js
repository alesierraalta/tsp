import { test } from 'node:test';
import assert from 'node:assert/strict';
import { distinctJobs, JobLabels } from '../src/catalog.js';
test('catalog order', () => { assert.deepEqual(distinctJobs([{id:'a'}, {id:'b'}, {id:'a'}]), [{id:'a'}, {id:'b'}]); });
test('label reuse', () => { let calls=0; const labels=new JobLabels(4, id => { calls++; return id.toUpperCase(); }); assert.equal(labels.label('a'),'A'); assert.equal(labels.label('a'),'A'); assert.equal(calls,1); });
