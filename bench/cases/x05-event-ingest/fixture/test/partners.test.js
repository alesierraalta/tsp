import test from 'node:test';
import assert from 'node:assert/strict';
import { PartnerBook, definePartner, partnerRow, partnerRows, PARTNER_KINDS, DEFAULT_FIELDS, PRIVATE_PATHS, blockedFields, checksumOf, renderPartnerRows, partnerManifest, bundleFiles, partnerSummary } from '../src/reports/partners.js';
import { clock } from './helpers.js';

const events = [
  { id: 'evt_1', name: 'page_view', at: '2026-02-09T10:00:00.000Z', properties: { path: '/pricing' }, context: { user: 'usr_1', referrer: 'https://news.example.com' } },
  { id: 'evt_2', name: 'purchase', at: '2026-02-09T11:00:00.000Z', properties: { path: '/checkout' }, context: { user: 'usr_2' } },
];

test('a partner declares the fields it may see and nothing else', () => {
  const partner = definePartner({ name: 'Beta Metrics', kind: 'reseller', fields: ['name', 'at', 'properties.path'] }, { clock });
  assert.equal(partner.id, 'beta-metrics');
  assert.equal(partner.kind, 'reseller');
  assert.deepEqual(partner.columns, ['name', 'at', 'properties_path']);
  assert.deepEqual(definePartner({ name: 'Plain' }, { clock }).fields, DEFAULT_FIELDS);
  assert.deepEqual(definePartner({ name: 'Plain', kind: 'nope' }, { clock }).kind, 'analytics');
  assert.deepEqual(PARTNER_KINDS, ['agency', 'reseller', 'analytics']);
  assert.throws(() => definePartner({ name: '' }, { clock }), /needs a name/);
  assert.throws(() => definePartner({ name: 'Sneaky', fields: ['name', 'context.session'] }, { clock }), (error) => error.code === 'partner_field_blocked');
  assert.throws(
    () => definePartner({ name: 'Wide', fields: Array.from({ length: 13 }, (_, index) => `properties.f${index}`) }, { clock }),
    /at most 12 fields/,
  );
  assert.deepEqual(blockedFields(), PRIVATE_PATHS);
});

test('a row is built from the declared paths and never from the rest of the event', () => {
  const partner = definePartner({ name: 'Beta Metrics', fields: ['name', 'at', 'properties.path'] }, { clock });
  const row = partnerRow(partner, events[0]);
  assert.deepEqual(row, { name: 'page_view', at: '2026-02-09T10:00:00.000Z', properties_path: '/pricing' });
  assert.equal(Object.keys(row).some((column) => column.includes('user')), false);
  assert.equal(partnerRows(partner, events).length, 2);
  const pathOnly = definePartner({ name: 'Paths', fields: ['properties.path'] }, { clock: () => new Date('2026-02-10T12:00:00.000Z') });
  assert.deepEqual(partnerRow(pathOnly, { id: 'evt_3' }), { properties_path: null });
});

test('a rendered export has one column per declared field', () => {
  const partner = definePartner({ name: 'Beta Metrics', fields: ['name', 'at'] }, { clock });
  const rows = partnerRows(partner, events);
  assert.equal(renderPartnerRows(partner, rows, 'csv'), 'name,at\npage_view,2026-02-09T10:00:00.000Z\npurchase,2026-02-09T11:00:00.000Z\n');
  assert.equal(renderPartnerRows(partner, rows, 'ndjson').split('\n').filter(Boolean).length, 2);
  assert.equal(renderPartnerRows(partner, rows, 'json').includes('"columns"'), true);
  assert.throws(() => renderPartnerRows(partner, rows, 'xml'), /unknown partner format/);
  const manifest = partnerManifest(partner, rows, 'csv');
  assert.equal(manifest.rows, 2);
  assert.equal(manifest.format, 'csv');
  assert.equal(manifest.checksum.length, 64);
  assert.equal(manifest.checksum, checksumOf(renderPartnerRows(partner, rows, 'csv')));
});

test('a partner book keeps its exports and their manifests', () => {
  const book = new PartnerBook({ clock });
  const partner = book.define({ name: 'Beta Metrics', fields: ['name', 'at'], tenants: ['acme'] });
  assert.throws(() => book.define({ name: 'Beta Metrics' }), /already exists/);
  assert.equal(book.get(partner.id).name, 'Beta Metrics');
  assert.throws(() => book.require('nope'), /does not exist/);
  assert.equal(book.forTenant('acme').length, 1);
  assert.equal(book.forTenant('globex').length, 0);
  book.assign(partner.id, 'globex');
  assert.deepEqual(book.get(partner.id).tenants, ['acme', 'globex']);
  assert.deepEqual(partnerSummary(partner, events).rows, 2);

  const exported = book.export(partner.id, events, { format: 'csv' });
  assert.equal(exported.name, 'beta-metrics-20260210-001.csv');
  assert.equal(exported.manifest.rows, 2);
  assert.equal(exported.text.startsWith('name,at\n'), true);
  assert.equal(book.file(partner.id, exported.name).name, exported.name);
  assert.equal(book.file('other', exported.name), undefined);
  assert.equal(book.filesOf(partner.id).length, 1);
  assert.equal(book.history(partner.id).length, 1);
  assert.deepEqual(book.stats(), { partners: 1, files: 1, exports: 1 });
});

test('a bundle writes the same rows in chunks with a checksum per chunk', () => {
  const partner = definePartner({ name: 'Beta Metrics', fields: ['name'] }, { clock });
  const bundle = bundleFiles(partner, partnerRows(partner, events), { format: 'ndjson', size: 1 });
  assert.equal(bundle.files.length, 2);
  assert.deepEqual(bundle.files.map((file) => file.name), ['beta-metrics-001.ndjson', 'beta-metrics-002.ndjson']);
  assert.equal(bundle.rows, 2);
  assert.equal(bundle.files[0].checksum.length, 64);
  assert.equal(bundle.checksum, checksumOf(bundle.files.map((file) => file.checksum).join('')));
  assert.equal(bundle.bytes, bundle.files[0].bytes + bundle.files[1].bytes);
  assert.equal(bundleFiles(partner, [], { format: 'csv' }).files.length, 0);
});
