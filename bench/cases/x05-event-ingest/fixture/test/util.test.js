import test from 'node:test';
import assert from 'node:assert/strict';
import { slugify, singleLine, maskSecret, snakeCase, initials, truncate, splitCsv, isBlank } from '../src/util/text.js';
import { dayBucket, hourBucket, toIso, isWithin, diffSeconds, rangeOf, parseWindow, addDays, alignedWindow } from '../src/util/time.js';
import { clone, stableStringify, hasUnsafeKey, flatten, deepEqual, parseSafe } from '../src/util/json.js';
import { groupBy, countBy, paginate, chunk, sortBy, diffIds, tally, windowed, spread, mode } from '../src/util/collections.js';
import { clamp, percentOf, roundTo, percentChange, budgetShare, sumCentsOf, bucketOfValue } from '../src/util/numbers.js';
import { isId, hexOf, idPrefix, batchId, newId } from '../src/util/ids.js';
import { badRequest, forbidden, notFound, statusOf, codeOf, isIngestError } from '../src/util/errors.js';

test('text helpers normalise and mask what a caller typed', () => {
  assert.equal(slugify('Café del Mar!'), 'cafe-del-mar');
  assert.equal(singleLine('  two\n lines  '), 'two lines');
  assert.equal(snakeCase('pageViewCount'), 'page_view_count');
  assert.equal(initials('ada lovelace byron'), 'AL');
  assert.equal(truncate('abcdef', 4), 'abc…');
  assert.equal(truncate('abc', 10), 'abc');
  assert.deepEqual(splitCsv('a, b,,c'), ['a', 'b', 'c']);
  assert.equal(isBlank('   '), true);
  assert.equal(isBlank('x'), false);
  assert.equal(maskSecret('ing_1234567890abcdef').startsWith('ing_'), true);
  assert.equal(maskSecret('short').includes('s'), false);
});

test('time helpers bucket instants the way the reports group them', () => {
  assert.equal(hourBucket('2026-02-09T10:34:12.000Z'), '2026-02-09T10:00:00.000Z');
  assert.equal(dayBucket('2026-02-09T23:59:00.000Z'), '2026-02-09');
  assert.equal(toIso('2026-02-09'), '2026-02-09T00:00:00.000Z');
  assert.equal(toIso('nonsense'), undefined);
  assert.equal(isWithin('2026-02-09T10:00:00.000Z', '2026-02-09T09:00:00.000Z', '2026-02-09T11:00:00.000Z'), true);
  assert.equal(isWithin('2026-02-09T12:00:00.000Z', '2026-02-09T09:00:00.000Z', '2026-02-09T11:00:00.000Z'), false);
  assert.equal(diffSeconds('2026-02-09T10:00:00.000Z', '2026-02-09T10:05:00.000Z'), 300);
  assert.deepEqual(rangeOf('2026-02-09T10:00:00.000Z', '2026-02-09T12:00:00.000Z', 'hour'), [
    '2026-02-09T10:00:00.000Z',
    '2026-02-09T11:00:00.000Z',
    '2026-02-09T12:00:00.000Z',
  ]);
  assert.equal(parseWindow('30m'), 30 * 60 * 1000);
  assert.equal(parseWindow('2d'), 2 * 24 * 60 * 60 * 1000);
  assert.equal(addDays('2026-02-09T00:00:00.000Z', 1).toISOString(), '2026-02-10T00:00:00.000Z');
  assert.deepEqual(alignedWindow('2026-02-09T10:34:12.000Z', 'hour', 2), {
    from: '2026-02-09T10:00:00.000Z',
    to: '2026-02-09T12:00:00.000Z',
  });
});

test('json helpers clone, flatten and refuse unsafe keys', () => {
  const source = { a: { b: 1 }, c: [1, 2] };
  const copy = clone(source);
  copy.a.b = 2;
  assert.equal(source.a.b, 1);
  assert.equal(stableStringify({ b: 1, a: 2 }), '{\n  "a": 2,\n  "b": 1\n}');
  assert.equal(hasUnsafeKey({ ok: 1 }), false);
  assert.equal(hasUnsafeKey({ nested: { __proto__: { polluted: true } } }), false);
  assert.equal(hasUnsafeKey({ ['__proto__']: { polluted: true } }), true);
  assert.deepEqual(flatten({ a: { b: 1 }, c: 2 }), { 'a.b': 1, c: 2 });
  assert.equal(deepEqual({ a: [1] }, { a: [1] }), true);
  assert.equal(deepEqual({ a: 1 }, { a: 2 }), false);
  assert.equal(parseSafe('not json', 'fallback'), 'fallback');
});

test('collections group, page and diff rows', () => {
  const rows = [
    { id: 'a', kind: 'x', n: 1 },
    { id: 'b', kind: 'x', n: 3 },
    { id: 'c', kind: 'y', n: 2 },
  ];
  const groups = groupBy(rows, (row) => row.kind);
  assert.equal(groups.get('x').length, 2);
  assert.equal(countBy(rows, (row) => row.kind).get('y'), 1);
  const page = paginate(rows, { limit: 2, offset: 1 });
  assert.deepEqual(page.items.map((row) => row.id), ['b', 'c']);
  assert.equal(page.has_more, false);
  assert.equal(chunk(rows, 2).length, 2);
  assert.deepEqual(sortBy(rows, (row) => row.n, 'desc').map((row) => row.id), ['b', 'c', 'a']);
  assert.deepEqual(diffIds(['a', 'b'], ['b', 'c']), { added: ['c'], removed: ['a'], kept: ['b'] });
  assert.deepEqual(tally(['a', 'a', 'b']), [['a', 2], ['b', 1]]);
  assert.deepEqual(windowed([1, 2, 3], 2).length, 2);
  assert.deepEqual(spread(rows, (row) => row.n), { min: 1, max: 3, range: 2 });
  assert.deepEqual(mode(['a', 'b', 'b']), { value: 'b', count: 2 });
});

test('number helpers keep money and percentages whole', () => {
  assert.equal(clamp(5, 0, 3), 3);
  assert.equal(percentOf(1, 4), 25);
  assert.equal(percentOf(1, 0), 0);
  assert.equal(roundTo(1.23456, 3), 1.235);
  assert.equal(percentChange(10, 12), 20);
  assert.equal(budgetShare(50, 200), 25);
  assert.equal(budgetShare(1, 0), 0);
  assert.equal(sumCentsOf([1.9, 2.2]), 3);
  assert.equal(bucketOfValue(7, [5, 10, 20]), 10);
});

test('ids are shaped the way the routes look for them', () => {
  const id = newId('rsv');
  assert.equal(id.startsWith('rsv_'), true);
  assert.equal(isId(id, 'rsv'), true);
  assert.equal(isId(id, 'job'), false);
  assert.equal(idPrefix('bat_abc'), 'bat');
  assert.equal(hexOf('acme', 8).length, 8);
  assert.equal(batchId('acme', 7), `bat_${hexOf('acme', 6)}000007`);
});

test('errors carry the code and status the response uses', () => {
  const error = badRequest('invalid_rows', 'rows must be a whole number', { limit: 10 });
  assert.equal(isIngestError(error), true);
  assert.equal(statusOf(error), 400);
  assert.equal(codeOf(error), 'invalid_rows');
  assert.deepEqual(error.toJSON(), { error: 'invalid_rows', message: 'rows must be a whole number', details: { limit: 10 } });
  assert.equal(statusOf(forbidden('nope')), 403);
  assert.equal(statusOf(notFound('event')), 404);
  assert.equal(statusOf(new TypeError('boom')), 500);
});
