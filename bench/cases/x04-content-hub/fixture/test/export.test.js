import test from 'node:test';
import assert from 'node:assert/strict';
import { toCsv, parseCsv, escapeCell, csvSummary } from '../src/export/csv.js';
import { Exporter, flattenEntry, exportPlan, splitRows, exportFileName, collectFields } from '../src/export/exporters.js';
import { Importer, detectFormat, recordsFrom } from '../src/export/importer.js';
import { sitemap, rssFeed, sitemapIndex, robotsTxt, documentCount } from '../src/export/sitemap.js';
import { facetCounts, applyFacets, facetSelection, rangeFacet, facetSummary } from '../src/search/facets.js';
import { SearchIndex, moreLike, mostCommon, indexHealth } from '../src/search/index.js';
import { tokens, tokenize, stem, snippet, vocabularySize, queryTerms } from '../src/search/tokenize.js';
import { MemoryStore } from '../src/store/memory.js';
import { RevisionLog } from '../src/store/revisions.js';
import { EntryService } from '../src/domain/entries.js';
import { SchemaRegistry, defaultSchemas } from '../src/domain/schemas.js';
import { defineContentType } from '../src/domain/content-types.js';
import { tempRoots, cleanup } from './helpers.js';

function world() {
  const store = new MemoryStore();
  const entries = new EntryService({ store, revisions: new RevisionLog() });
  const schemas = new SchemaRegistry(defaultSchemas().map((input) => defineContentType(input)));
  return { store, entries, schemas };
}

test('csv escapes and round-trips', () => {
  assert.equal(escapeCell('a,b'), '"a,b"');
  assert.equal(escapeCell('say "hi"'), '"say ""hi"""');
  const text = toCsv([{ a: 1, b: 'x,y' }], ['a', 'b']);
  assert.equal(text, 'a,b\n1,"x,y"\n');
  assert.deepEqual(parseCsv(text).rows, [{ a: '1', b: 'x,y' }]);
  assert.deepEqual(csvSummary('a,b\n1,2\n'), { fields: 2, rows: 1 });
});

test('entries flatten to export rows', async () => {
  const { entries, schemas } = world();
  const article = schemas.get('article');
  entries.create(article, { values: { title: 'Exported', body: 'Body', topic: 'news' } });
  const row = flattenEntry(entries.list(article)[0], ['title', 'topic']);
  assert.equal(row.title, 'Exported');
  assert.equal(row.status, 'draft');
  assert.deepEqual(collectFields([row]).includes('slug'), true);
});

test('exporters write json, csv and ndjson', async () => {
  const roots = await tempRoots();
  try {
    const { entries, schemas } = world();
    const article = schemas.get('article');
    entries.create(article, { values: { title: 'One', body: 'B', topic: 'news' } });
    const exporters = new Exporter({ entries, schemas, root: roots.exports });
    const json = await exporters.exportType('article', 'json');
    assert.equal(json.rows, 1);
    assert.match(await exporters.read('article.json'), /"exported_at"/);
    await exporters.exportType('article', 'csv');
    assert.match(await exporters.read('article.csv'), /^id,slug,locale/);
    await exporters.exportType('article', 'ndjson', { name: 'article.ndjson' });
    assert.equal((await exporters.list()).length, 3);
    assert.equal(exportPlan(exporters.rowsOf('article'), 'json').rows, 1);
    assert.deepEqual(splitRows([1, 2, 3], 2).length, 2);
    assert.match(exportFileName('article', 'csv', '2026-03-05T00:00:00.000Z'), /^article-20260305\.csv$/);
    await assert.rejects(() => exporters.exportType('missing', 'json'), /not found/);
    await assert.rejects(() => exporters.exportType('article', 'xml'), /must be one of/);
  } finally {
    await cleanup(roots);
  }
});

test('importers read csv, json and ndjson', async () => {
  const { entries, schemas } = world();
  const note = schemas.define({ name: 'Note', fields: [{ name: 'title', type: 'text', required: true }] });
  const importer = new Importer({ entries, schemas });
  assert.equal(detectFormat('a.csv'), 'csv');
  assert.throws(() => detectFormat('a.xlsx'), /unknown format/);
  assert.equal(recordsFrom('a,b\n1,2\n', 'csv').length, 1);
  assert.equal(recordsFrom('[{"a":1}]', 'json').length, 1);
  assert.equal(recordsFrom('{"a":1}\n{"a":2}\n', 'ndjson').length, 2);
  const result = importer.importRecords(note, [{ title: 'Imported' }]);
  assert.equal(result.created, 1);
  assert.deepEqual(importer.summary(), { files: 1, created: 1, skipped: 0 });
  assert.equal(importer.validateOnly(note, [{}]).length, 1);
});

test('sitemap and rss list published entries', () => {
  const records = [
    { slug: 'a', status: 'published', updated_at: '2026-03-05T10:00:00.000Z', values: { title: 'A', body: 'B' } },
    { slug: 'b', status: 'draft', updated_at: '2026-03-05T10:00:00.000Z', values: {} },
  ];
  const xml = sitemap(records);
  assert.equal(documentCount(xml), 1);
  assert.match(xml, /<loc>https:\/\/example.invalid\/a<\/loc>/);
  assert.match(rssFeed(records), /<item>/);
  assert.match(sitemapIndex(['sitemap-1.xml']), /sitemap-1\.xml/);
  assert.match(robotsTxt({ disallow: '/admin' }), /disallow: \/admin/);
});

test('facets count and filter', () => {
  const records = [
    { values: { topic: 'news' } },
    { values: { topic: 'news' } },
    { values: { topic: 'guide' } },
  ];
  assert.deepEqual(facetCounts(records, 'topic'), [
    { value: 'news', count: 2 },
    { value: 'guide', count: 1 },
  ]);
  assert.deepEqual(facetSelection({ facet_topic: 'news', other: 'x' }), { topic: ['news'] });
  assert.equal(applyFacets(records, { topic: ['news'] }).length, 2);
  assert.equal(facetSummary({ topic: [{ value: 'a', count: 1 }] })[0].values, 1);
  assert.equal(rangeFacet([5, 50, 500])[0].count, 2);
});

test('the search index ranks documents', () => {
  const index = new SearchIndex();
  index.add({ id: 'e1', slug: 'hello-world', type: 'article', locale: 'en', values: { title: 'Hello world', body: 'world of content' } });
  index.add({ id: 'e2', slug: 'other-world', type: 'page', locale: 'en', values: { title: 'Other world', body: 'nothing here' } });
  const result = index.search('content');
  assert.equal(result.total, 1);
  assert.equal(result.hits[0].slug, 'hello-world');
  assert.equal(index.suggestions('wor').length, 1);
  assert.equal(index.stats().documents, 2);
  assert.equal(mostCommon(index)[0].documents >= 1, true);
  assert.equal(moreLike(index, 'e1').length, 1);
  assert.equal(indexHealth(index).documents, 2);
  index.remove('e2');
  assert.equal(index.stats().documents, 1);
});

test('tokenizing folds case and accents', () => {
  assert.deepEqual(tokenize('The Café, again'), ['cafe', 'again']);
  assert.deepEqual(tokens('stories'), ['story']);
  assert.equal(stem('boxes'), 'box');
  assert.equal(snippet('a needle in a haystack', 'needle').includes('needle'), true);
  assert.deepEqual(queryTerms('world world'), ['world']);
  assert.equal(vocabularySize(['alpha beta', 'beta gamma']), 3);
});
