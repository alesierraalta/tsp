import test from 'node:test';
import assert from 'node:assert/strict';
import { Catalog } from '../src/catalog.js';
import { SearchService } from '../src/service.js';
import { readSettings, describeSettings, settingsEqual } from '../src/settings.js';
import { interpretQuery } from '../src/query.js';
import { formatResult, pageLinks } from '../src/presentation.js';
import { renderManifest, serializeEnvironment } from '../deploy/environment.mjs';

function catalog() {
  const store = new Catalog();
  store.put({ id: 'a', title: 'Blue birds', body: 'Garden guide', tags: ['nature'] });
  store.put({ id: 'b', title: 'Garden', body: 'Blue flowers', tags: ['nature'] });
  store.put({ id: 'c', title: 'Private', body: 'Blue birds', visible: false });
  return store;
}

test('search ranking, pages and presentation', () => {
  const service = new SearchService(catalog(), readSettings({ SEARCH_PAGE_SIZE: '1' }));
  const result = service.search('  Blue\n  ', { tag: 'nature' });
  assert.equal(result.query.text, 'Blue');
  assert.equal(result.total, 2);
  assert.equal(result.rows[0].id, 'a');
  assert.equal(result.rows[0].score, 3);
  assert.match(pageLinks(result).next, /page=2/);
  const next = service.search('Blue', { page: 2 });
  assert.equal(next.rows[0].id, 'b');
  assert.match(pageLinks(next).previous, /page=1/);
  assert.equal(JSON.parse(formatResult(result)).total, 2);
  assert.match(formatResult(result, 'text'), /Blue birds/);
  assert.throws(() => formatResult(result, 'xml'));
  assert.deepEqual(service.stats(), { requests: 2, returned: 2 });
  service.resetStats();
  assert.equal(service.stats().requests, 0);
  assert.throws(() => service.search('Blue', { page: 0 }));
});

test('catalog edits and independent returned documents', () => {
  const store = catalog();
  assert.equal(store.size, 3);
  assert.equal(store.list().length, 2);
  assert.equal(store.list({ includeHidden: true }).length, 3);
  const row = store.get('a');
  row.tags.push('other');
  assert.deepEqual(store.get('a').tags, ['nature']);
  assert.equal(store.remove('a'), true);
  assert.equal(store.get('a'), null);
  assert.throws(() => store.put({ id: 'x', title: 3 }));
  assert.throws(() => store.put({ id: '' }));
  assert.throws(() => store.put({ id: 'x', title: 'X', body: '', tags: [3] }));
});

test('query options and settings validation', () => {
  const settings = readSettings({ SEARCH_MAX_QUERY_LENGTH: '4' });
  assert.equal(interpretQuery('Bluebird', settings).text, 'Blue');
  assert.throws(() => interpretQuery(null, settings));
  assert.throws(() => readSettings({ SEARCH_PAGE_SIZE: '0' }));
  assert.throws(() => readSettings({ SEARCH_PAGE_SIZE: 'many' }));
  assert.throws(() => readSettings({ SEARCH_SORT: 'random' }));
  assert.equal(describeSettings(settings).query.maximum, 4);
  assert.equal(settingsEqual(settings, readSettings({ SEARCH_MAX_QUERY_LENGTH: '4' })), true);
  const store = catalog();
  const any = new SearchService(store, readSettings({ SEARCH_MATCH_MODE: 'any' }));
  assert.equal(any.search('birds flowers').total, 2);
  const phrase = new SearchService(store, readSettings({ SEARCH_MATCH_MODE: 'phrase' }));
  assert.equal(phrase.search('Blue birds').total, 1);
  const title = new SearchService(store, readSettings({ SEARCH_SORT: 'title' }));
  assert.equal(title.search('').rows[0].id, 'a');
});

test('deployment manifest accepts ordinary overrides', () => {
  const manifest = renderManifest({ SEARCH_PAGE_SIZE: '5', SEARCH_SORT: 'title', OTHER: 'ignored' });
  assert.deepEqual(manifest.environment, { SEARCH_PAGE_SIZE: '5', SEARCH_SORT: 'title' });
  assert.equal(manifest.settings.results.pageSize, 5);
  assert.equal(JSON.parse(serializeEnvironment({ SEARCH_MAX_QUERY_LENGTH: '60' })).SEARCH_MAX_QUERY_LENGTH, '60');
  assert.throws(() => renderManifest({}, 'invalid name'));
  assert.throws(() => renderManifest({ SEARCH_PAGE_SIZE: 5 }));
});
