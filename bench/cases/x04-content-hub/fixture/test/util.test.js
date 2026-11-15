import test from 'node:test';
import assert from 'node:assert/strict';
import { slugify, truncate, titleCase, kebabCase, camelCase, splitCsv, wordCount, readingTime, maskEmail, ordinal, headingsOf, initials, summarize } from '../src/util/text.js';
import { stableStringify, deepEqual, pick, omit, flatten, clone, hasUnsafeKey, mergeSafe, setAt, valueAt } from '../src/util/json.js';
import { addDays, dayOf, durationMinutes, isPast, nextRun, relative, isoWeek, parseDuration, formatDuration, bucketByDay } from '../src/util/time.js';
import { groupBy, chunk, paginate, sortBy, uniqueBy, tally, diffLists, rotate, sliding } from '../src/util/collections.js';
import { AppError, codeOf, statusOf, invalidField, notFound, unsafePath, tooLarge } from '../src/util/errors.js';
import { createLogger, redact } from '../src/util/log.js';
import { newId, shortId, encodeBase36, decodeBase36, isId, idPrefix } from '../src/util/ids.js';

test('slugify folds accents and collapses separators', () => {
  assert.equal(slugify('Café del Mar!'), 'cafe-del-mar');
  assert.equal(slugify('   '), 'untitled');
  assert.equal(slugify('A/B testing'), 'a-b-testing');
});

test('text helpers keep values inside their limits', () => {
  assert.equal(truncate('abcdefghij', 5), 'abcd…');
  assert.equal(titleCase('hello wide world'), 'Hello Wide World');
  assert.equal(kebabCase('contentType'), 'content-type');
  assert.equal(camelCase('content_type'), 'contentType');
  assert.deepEqual(splitCsv('a, b,,c'), ['a', 'b', 'c']);
  assert.equal(wordCount('one two  three'), 3);
  assert.equal(readingTime('word '.repeat(440)), 2);
  assert.equal(maskEmail('sam@example.com'), 'sa*@example.com');
  assert.equal(ordinal(2), '2nd');
  assert.equal(ordinal(11), '11th');
  assert.equal(initials('Ada Lovelace'), 'AL');
  assert.equal(summarize('One. Two. Three.', 2), 'One. Two.');
});

test('headings are read from a markdown document', () => {
  const headings = headingsOf('# Title\n\n## Section\n\ntext\n');
  assert.deepEqual(headings, [
    { level: 1, text: 'Title' },
    { level: 2, text: 'Section' },
  ]);
});

test('json helpers clone, pick and deep-compare', () => {
  const source = { a: 1, b: { c: [1, 2] }, d: undefined };
  const copy = clone(source);
  copy.b.c.push(3);
  assert.deepEqual(source.b.c, [1, 2]);
  assert.deepEqual(pick(source, ['a', 'missing']), { a: 1 });
  assert.deepEqual(omit(source, ['a']), { b: { c: [1, 2] }, d: undefined });
  assert.equal(deepEqual(source, clone(source)), true);
  assert.deepEqual(flatten({ a: { b: 1 } }), { 'a.b': 1 });
  assert.equal(stableStringify({ b: 1, a: 2 }), '{\n  "a": 2,\n  "b": 1\n}');
});

test('unsafe keys are refused or dropped by the merge helpers', () => {
  assert.equal(hasUnsafeKey({ a: { b: 1 } }), false);
  assert.equal(hasUnsafeKey(JSON.parse('{"__proto__":{"x":1}}')), true);
  const merged = mergeSafe({ a: { b: 1 } }, { a: { c: 2 } });
  assert.deepEqual(merged, { a: { b: 1, c: 2 } });
  assert.equal(setAt({}, 'a.b', 1).a.b, 1);
  assert.equal(valueAt({ a: { b: 2 } }, 'a.b'), 2);
});

test('time helpers work on iso days', () => {
  assert.equal(dayOf('2026-03-05T10:00:00.000Z'), '2026-03-05');
  assert.equal(dayOf(addDays('2026-03-05T00:00:00.000Z', 2)), '2026-03-07');
  assert.equal(durationMinutes('2026-03-05T10:00:00.000Z', '2026-03-05T10:45:00.000Z'), 45);
  assert.equal(isPast('2020-01-01T00:00:00.000Z'), true);
  assert.equal(nextRun('2026-03-05T10:00:00.000Z', 30).toISOString(), '2026-03-05T10:30:00.000Z');
  assert.equal(relative('2026-03-05T10:00:00.000Z', () => new Date('2026-03-05T10:00:00.000Z')), 'now');
  assert.deepEqual(isoWeek('2026-01-01T00:00:00.000Z'), { year: 2026, week: 1 });
  assert.equal(parseDuration('2h'), 120);
  assert.equal(formatDuration(90), '1h 30m');
  assert.deepEqual(bucketByDay(['2026-03-05T01:00:00.000Z', '2026-03-05T22:00:00.000Z']), [{ day: '2026-03-05', count: 2 }]);
});

test('collection helpers keep order and shape', () => {
  const items = [{ id: 'a', n: 2 }, { id: 'b', n: 1 }, { id: 'a', n: 3 }];
  assert.equal(groupBy(items, (item) => item.id).get('a').length, 2);
  assert.equal(chunk([1, 2, 3, 4, 5], 2).length, 3);
  assert.equal(paginate([1, 2, 3, 4], { limit: 2, offset: 0 }).has_more, true);
  assert.deepEqual(sortBy(items, (item) => item.n).map((item) => item.n), [1, 2, 3]);
  assert.equal(uniqueBy(items, (item) => item.id).length, 2);
  assert.deepEqual(tally(['a', 'b', 'a']), [['a', 2], ['b', 1]]);
  assert.deepEqual(diffLists(['a', 'b'], ['b', 'c']), { added: ['c'], removed: ['a'], kept: ['b'] });
  assert.deepEqual(rotate([1, 2, 3], 1), [2, 3, 1]);
  assert.deepEqual(sliding([1, 2, 3], 2), [[1, 2], [2, 3]]);
});

test('errors carry a status and a json body', () => {
  const error = invalidField('title', 'is required');
  assert.equal(error.status, 400);
  assert.equal(codeOf(error), 'invalid_field');
  assert.deepEqual(error.toJSON(), { error: 'invalid_field', message: 'title is required', details: { field: 'title' } });
  assert.equal(statusOf(notFound('entry')), 404);
  assert.equal(unsafePath('../x').code, 'unsafe_path');
  assert.equal(tooLarge('big').status, 413);
  assert.equal(statusOf(new Error('boom')), 500);
  assert.ok(new AppError('x', 'y', { status: 418 }).toJSON().error === 'x');
});

test('the logger redacts secrets and keeps entries', () => {
  const lines = [];
  const logger = createLogger({ level: 'debug', sink: (line) => lines.push(line) });
  logger.info('write', { token: 'abc', user: 'sam' });
  assert.equal(lines.length, 1);
  assert.equal(logger.entries[0].token, '[redacted]');
  assert.deepEqual(redact({ authorization: 'x' }), { authorization: '[redacted]' });
});

test('ids are prefixed and reversible', () => {
  const id = newId('entry');
  assert.equal(isId(id, 'entry'), true);
  assert.equal(idPrefix(id), 'entry');
  assert.equal(shortId('job').length, 12);
  assert.equal(decodeBase36(encodeBase36(12345)), 12345);
  assert.equal(encodeBase36(36), '10');
});
