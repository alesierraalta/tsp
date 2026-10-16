import test from 'node:test';
import assert from 'node:assert/strict';
import { Service } from '../src/service.js';
import { Store } from '../src/store.js';
import { Client } from '../src/client.js';
import { Audit } from '../src/audit.js';
import { encodePatch, decodePatch, applyPatch, decodeSnapshot } from '../src/codec.js';
import { queryRows, projectRows } from '../src/query.js';
import { csvCell, exportJSON, describeExport, chunks } from '../src/export.js';
import { selection } from '../src/schema.js';
import { compactRecord, trimRecord, fieldCounts } from '../src/normalize.js';

test('directory lifecycle and audit', () => {
  const service = new Service();
  const client = service.client();
  client.create({ id: 'a', name: 'Alice', email: 'a@example.test', team: 'ops', note: 'active' });
  client.update('a', { name: 'Alicia' });
  assert.equal(client.read('a').email, 'a@example.test');
  assert.equal(client.read('a').name, 'Alicia');
  assert.equal(client.export(['id', 'name']), 'id,name\na,Alicia');
  assert.equal(decodeSnapshot(client.snapshot()).revision, 2);
  assert.equal(service.audit.forRecord('a').length, 2);
  assert.equal(service.audit.summary().actions.update, 1);
  client.remove('a');
  assert.throws(() => client.read('a'));
  assert.equal(service.audit.latest, 3);
});

test('codec and validation', () => {
  assert.deepEqual(decodePatch(encodePatch({ email: null, note: undefined })), { email: null });
  assert.equal(applyPatch({ id: 'a', email: 'x' }, { email: null }).email, null);
  assert.throws(() => encodePatch({ id: 'b' }));
  assert.throws(() => decodePatch('{"team":12}'));
  assert.throws(() => decodeSnapshot('{"rows":[],"revision":-1}'));
  assert.throws(() => selection(['name', 'name']));
  assert.throws(() => selection(['extra']));
  assert.throws(() => new Client(null));
  assert.deepEqual(compactRecord({ id: 'a', note: null }), { id: 'a' });
  assert.deepEqual(trimRecord({ name: ' Alice ', note: null }), { name: 'Alice', note: null });
  assert.equal(fieldCounts([{ id: 'a', note: null }]).id, 1);
  const store = new Store();
  store.create({ id: ' a ' });
  assert.equal(store.read('a').note, null);
  assert.throws(() => store.create({ id: 'a' }));
  assert.throws(() => store.create({ id: '', name: 'x' }));
  assert.throws(() => store.create({ id: 'b', note: 2 }));
  const copy = store.read('a'); copy.name = 'changed';
  assert.equal(store.read('a').name, null);
});

test('query and export utilities', () => {
  const rows = [{ id: 'b', name: 'Bee', team: 'ops' }, { id: 'a', name: 'Aye', team: 'ops' }];
  const result = queryRows(rows, { filter: { team: 'ops' }, limit: 1 });
  assert.equal(result.rows[0].id, 'a');
  assert.equal(result.next, 1);
  assert.equal(queryRows(rows, { direction: 'desc' }).rows[0].id, 'b');
  assert.deepEqual(projectRows(rows, ['name'])[0], { name: 'Bee' });
  assert.equal(exportJSON(rows, ['id']), '[{"id":"a"},{"id":"b"}]');
  assert.equal(describeExport(rows, ['id']).count, 2);
  assert.equal(csvCell('a,"b"'), '"a,""b"""');
  assert.equal(csvCell(null), '');
  assert.deepEqual(chunks('abcdef', 2), ['ab', 'cd', 'ef']);
  assert.throws(() => chunks('x', 0));
  assert.throws(() => queryRows(rows, { offset: -1 }));
  assert.throws(() => queryRows(rows, { filter: { unknown: 'x' } }));
  const service = new Service();
  service.client().create({ id: 'a' });
  assert.equal(service.query().total, 1);
  assert.throws(() => service.request({ method: 'OTHER' }));
});

test('event views are detached', () => {
  const audit = new Audit();
  audit.record('create', 'a', 1);
  audit.record('update', 'a', 2);
  const events = audit.since(1); events[0].action = 'other';
  assert.equal(audit.since(1)[0].action, 'update');
  assert.throws(() => audit.since(-1));
  assert.throws(() => audit.record('', 'a', 2));
});
