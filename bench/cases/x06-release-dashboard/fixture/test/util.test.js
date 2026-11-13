import test from 'node:test';
import assert from 'node:assert/strict';
import { singleLine, slugify, titleCase, snakeCase, camelCase, truncate, maskSecret, splitList, pluralize, joinWithAnd, alignTable } from '../src/util/text.js';
import { toInstant, toIso, dayBucket, hourBucket, weekKey, monthKey, addMinutes, addDays, diffMinutes, diffSeconds, isWithin, rangeOf, parseDuration, formatDuration, describeSpan } from '../src/util/time.js';
import { clone, stableStringify, pick, omit, flatten, byteSize, deepEqual, mergeDeep, diffKeys, hasUnsafeKey } from '../src/util/json.js';
import { groupBy, countBy, uniqueBy, chunk, sortBy, paginate, topBy, diffIds, windowed, spread, mode } from '../src/util/collections.js';
import { clamp, percentOf, roundTo, safeDivide, mean, median, quantile, formatNumber, formatBytes, percentChange, describeRate } from '../src/util/numbers.js';
import { newId, isId, idPrefix, shortHash, stableId, sequenceId } from '../src/util/ids.js';
import { parseUrl, hostOf, pathOf, originOf, isHttps, sameOrigin, withoutCredentials, withQuery, normalizeHost, describeTarget } from '../src/util/url.js';
import { badRequest, forbidden, notFound, conflict, statusOf, codeOf, isDashboardError } from '../src/util/errors.js';
import { ValidateError, requireString, requireEnum, requireInteger, clampInt, checkShape, asInteger } from '../src/util/validate.js';
import { createLogger, memorySink, levelOf, redactSecrets } from '../src/util/log.js';

test('text helpers shape the labels the views print', () => {
  assert.equal(singleLine('  two\n lines '), 'two lines');
  assert.equal(slugify('Checkout API v2!'), 'checkout-api-v2');
  assert.equal(titleCase('release dashboard'), 'Release Dashboard');
  assert.equal(snakeCase('releaseNotes'), 'release_notes');
  assert.equal(camelCase('release notes'), 'releaseNotes');
  assert.equal(truncate('abcdef', 4), 'abc…');
  assert.equal(maskSecret('rdk_1234567890abcdef').startsWith('rdk_'), true);
  assert.deepEqual(splitList('a, b ,,c'), ['a', 'b', 'c']);
  assert.equal(pluralize(2, 'approval'), '2 approvals');
  assert.equal(pluralize(1, 'approval'), '1 approval');
  assert.equal(joinWithAnd(['a', 'b', 'c']), 'a, b and c');
  assert.equal(alignTable([['a', 'bb'], ['cc', 'd']]).split('\n').length, 2);
});

test('time helpers bucket instants the way the reports group them', () => {
  assert.equal(hourBucket('2026-02-09T10:34:00.000Z'), '2026-02-09T10:00:00.000Z');
  assert.equal(dayBucket('2026-02-09T23:00:00.000Z'), '2026-02-09');
  assert.equal(weekKey('2026-02-09T10:00:00.000Z'), '2026-02-09');
  assert.equal(monthKey('2026-02-09T10:00:00.000Z'), '2026-02');
  assert.equal(toIso('2026-02-09'), '2026-02-09T00:00:00.000Z');
  assert.equal(toInstant('nonsense'), undefined);
  assert.equal(addMinutes('2026-02-09T10:00:00.000Z', 30).toISOString(), '2026-02-09T10:30:00.000Z');
  assert.equal(addDays('2026-02-09T00:00:00.000Z', 1).toISOString(), '2026-02-10T00:00:00.000Z');
  assert.equal(diffMinutes('2026-02-09T10:00:00.000Z', '2026-02-09T11:30:00.000Z'), 90);
  assert.equal(diffSeconds('2026-02-09T10:00:00.000Z', '2026-02-09T10:01:00.000Z'), 60);
  assert.equal(isWithin('2026-02-09T10:00:00.000Z', '2026-02-09T09:00:00.000Z', '2026-02-09T11:00:00.000Z'), true);
  assert.equal(rangeOf('2026-02-09T10:00:00.000Z', '2026-02-09T12:00:00.000Z', 'hour').length, 3);
  assert.equal(parseDuration('30m'), 1800000);
  assert.equal(formatDuration(90 * 60000), '1.5h');
  assert.deepEqual(describeSpan('2026-02-09T10:00:00.000Z', '2026-02-09T12:00:00.000Z').hours, 2);
});

test('json helpers clone, flatten and refuse unsafe keys', () => {
  const source = { a: { b: 1 } };
  const copy = clone(source);
  copy.a.b = 2;
  assert.equal(source.a.b, 1);
  assert.equal(stableStringify({ b: 1, a: 2 }), '{\n  "a": 2,\n  "b": 1\n}');
  assert.deepEqual(pick({ a: 1, b: 2 }, ['a']), { a: 1 });
  assert.deepEqual(omit({ a: 1, b: 2 }, ['a']), { b: 2 });
  assert.deepEqual(flatten({ a: { b: 1 }, c: 2 }), { 'a.b': 1, c: 2 });
  assert.equal(byteSize('abc'), 3);
  assert.equal(deepEqual({ a: [1] }, { a: [1] }), true);
  assert.equal(mergeDeep({ a: { b: 1 } }, { a: { c: 2 } }).a.c, 2);
  assert.deepEqual(diffKeys({ a: 1 }, { a: 2, b: 1 }), ['a', 'b']);
  assert.equal(hasUnsafeKey({ ['__proto__']: {} }), true);
});

test('collections group, page and diff rows', () => {
  const rows = [
    { id: 'a', service: 'x', n: 1 },
    { id: 'b', service: 'x', n: 3 },
    { id: 'c', service: 'y', n: 2 },
  ];
  assert.equal(groupBy(rows, (row) => row.service).get('x').length, 2);
  assert.equal(countBy(rows, (row) => row.service).get('y'), 1);
  assert.equal(uniqueBy([{ id: 1 }, { id: 1 }], (row) => row.id).length, 1);
  assert.equal(chunk(rows, 2).length, 2);
  assert.deepEqual(sortBy(rows, (row) => row.n, 'desc').map((row) => row.id), ['b', 'c', 'a']);
  assert.deepEqual(paginate(rows, { limit: 2, offset: 1 }).items.map((row) => row.id), ['b', 'c']);
  assert.equal(topBy(rows, (row) => row.n, 1)[0].id, 'b');
  assert.deepEqual(diffIds(['a'], ['b']), { added: ['b'], removed: ['a'], kept: [] });
  assert.equal(windowed([1, 2, 3], 2).length, 2);
  assert.deepEqual(spread(rows, (row) => row.n), { min: 1, max: 3, range: 2 });
  assert.deepEqual(mode(['a', 'a', 'b']), { value: 'a', count: 2 });
});

test('number helpers keep percentages and durations readable', () => {
  assert.equal(clamp(7, 0, 3), 3);
  assert.equal(percentOf(1, 4), 25);
  assert.equal(roundTo(1.2345, 3), 1.235);
  assert.equal(safeDivide(1, 0, -1), -1);
  assert.equal(mean([1, 2, 3]), 2);
  assert.equal(median([1, 3, 2]), 2);
  assert.equal(quantile([1, 2, 3, 4], 0.5), 3);
  assert.equal(formatNumber(12345), '12,345');
  assert.equal(formatBytes(2048), '2.0 KiB');
  assert.equal(percentChange(10, 12), 20);
  assert.equal(describeRate(1, 4), '25%');
});

test('ids are shaped the way the routes look for them', () => {
  const id = newId('rel');
  assert.equal(id.startsWith('rel_'), true);
  assert.equal(isId(id, 'rel'), true);
  assert.equal(isId(id, 'dep'), false);
  assert.equal(idPrefix('rel_abc'), 'rel');
  assert.equal(shortHash('checkout', 6).length, 6);
  assert.equal(stableId('svc', 'checkout').startsWith('svc_'), true);
  assert.equal(sequenceId('job', 'digest', 3).endsWith('00003'), true);
});

test('url helpers read the host the request actually reaches', () => {
  assert.equal(hostOf('https://hooks.example.com/relay'), 'hooks.example.com');
  assert.equal(pathOf('https://hooks.example.com/relay?a=1'), '/relay?a=1');
  assert.equal(originOf('https://hooks.example.com/relay'), 'https://hooks.example.com');
  assert.equal(isHttps('https://hooks.example.com'), true);
  assert.equal(isHttps('http://hooks.example.com'), false);
  assert.equal(sameOrigin('https://a.example.com/x', 'https://a.example.com/y'), true);
  assert.equal(withoutCredentials('https://user:pass@a.example.com/x'), 'https://a.example.com/x');
  assert.equal(withQuery('https://a.example.com/x', { page: 2 }), 'https://a.example.com/x?page=2');
  assert.equal(normalizeHost('Hooks.Example.COM.'), 'hooks.example.com');
  assert.equal(describeTarget('https://user@127.0.0.1:9000/x').host, '127.0.0.1');
  assert.throws(() => parseUrl('not a url'), /not an absolute URL/);
});

test('errors carry the code and status the responses use', () => {
  const error = badRequest('invalid_target', 'a target must use https', { field: 'url' });
  assert.equal(isDashboardError(error), true);
  assert.equal(statusOf(error), 400);
  assert.equal(codeOf(error), 'invalid_target');
  assert.deepEqual(error.toJSON(), { error: 'invalid_target', message: 'a target must use https', details: { field: 'url' } });
  assert.equal(statusOf(forbidden('nope')), 403);
  assert.equal(statusOf(notFound('release')), 404);
  assert.equal(statusOf(conflict('state', 'nope')), 409);
  assert.equal(statusOf(new TypeError('boom')), 500);
});

test('validation helpers describe which field is wrong', () => {
  assert.equal(requireString(' checkout ', 'service', { max: 20 }), 'checkout');
  assert.throws(() => requireString('', 'service'), ValidateError);
  assert.equal(requireEnum('production', ['development', 'production'], 'environment'), 'production');
  assert.throws(() => requireEnum('mars', ['development'], 'environment'), /must be one of/);
  assert.equal(requireInteger('3', 'steps', { min: 1, max: 5 }), 3);
  assert.throws(() => requireInteger('6', 'steps', { max: 5 }), /at most 5/);
  assert.equal(clampInt('99', 1, 10, 3), 10);
  assert.equal(checkShape({ version: '1.0.0' }, { version: { required: true, type: 'string' } }).ok, true);
  assert.equal(checkShape({}, { version: { required: true } }).problems[0].field, 'version');
  assert.equal(asInteger('7', 0), 7);
});

test('the logger writes one structured line per record', () => {
  const sink = memorySink();
  const logger = createLogger({ level: 'info', sink: sink.sink, now: () => new Date('2026-02-10T12:00:00.000Z') });
  logger.debug('skipped', { detail: 1 });
  logger.info('request', { path: '/' });
  logger.error('failed', { message: 'boom' });
  assert.equal(sink.records().length, 2);
  assert.equal(sink.records()[0].level, 'info');
  assert.equal(levelOf('warn') > levelOf('info'), true);
  assert.deepEqual(redactSecrets({ secret: 'x', path: '/' }), { secret: '***', path: '/' });
});
