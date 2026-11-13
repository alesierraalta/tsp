import test from 'node:test';
import assert from 'node:assert/strict';
import { defineEventType, SchemaBook, validateEvent, defaultTypes } from '../src/ingest/schemas.js';
import { Intake } from '../src/ingest/intake.js';
import { DedupeWindow } from '../src/ingest/dedupe.js';
import { splitLines, parseNdjson, planNdjson, NdjsonIntake, toNdjson } from '../src/ingest/ndjson.js';
import { Replayer, replayPlan, shiftTimestamps } from '../src/ingest/replay.js';
import { planBatch, assertWithinLimits, limitReport, remainingCapacity, exceedsAny } from '../src/ingest/limits.js';
import { normalizeEnvelope, envelopeSummary, splitEnvelope, upgradeEnvelope, acceptedEnvelope } from '../src/ingest/envelope.js';
import { enrichEvent, humanShare, deviceBreakdown, countryBreakdown, referrerHost, botShare } from '../src/ingest/enrich.js';
import { histogram, percentileSummary, sampleTotals, sampleSketch, truncateSamples } from '../src/ingest/sampling.js';
import { validateProperties, checkField, coerceProperties, fieldSummary, validateBatch } from '../src/ingest/validate.js';
import { MemoryStore } from '../src/store/memory.js';
import { SeriesStore } from '../src/store/series.js';
import { PlanBook } from '../src/quota/plans.js';
import { clock } from './helpers.js';

function build() {
  const store = new MemoryStore({ clock });
  const plans = new PlanBook();
  const schemas = new SchemaBook(defaultTypes());
  const dedupe = new DedupeWindow({ limit: 100 });
  const series = new SeriesStore({ clock });
  const intake = new Intake({ store, plans, schemas, dedupe, series, clock });
  return { store, plans, schemas, dedupe, series, intake };
}

test('an event type is declared once and validated field by field', () => {
  const type = defineEventType({ name: 'signup', kind: 'signup', fields: [{ name: 'plan', type: 'string', required: true, max: 20 }] });
  assert.equal(type.name, 'signup');
  assert.deepEqual(type.required, ['plan']);
  assert.throws(() => defineEventType({ name: 'Signup' }), /snake_case/);
  assert.throws(() => defineEventType({ name: 'signup2', fields: [{ name: 'plan', type: 'object' }] }), /unknown type/);
  const book = new SchemaBook(defaultTypes());
  book.define({ name: 'click', fields: [{ name: 'target', type: 'string' }] });
  assert.equal(book.size(), 3);
  assert.equal(book.list().find((entry) => entry.name === 'click').fields, 1);
  assert.throws(() => book.define({ name: 'click' }), /exists/);
  assert.deepEqual(validateProperties({ name: 'click', fields: [{ name: 'target', type: 'string' }] }, { other: 1 }), [
    { field: 'other', message: 'other is not declared by click' },
  ]);
  assert.equal(checkField({ name: 'n', type: 'number', min: 2 }, 1), 'n is below 2');
  assert.equal(coerceProperties({ fields: [{ name: 'n', type: 'number' }] }, { n: '42' }).n, 42);
  assert.equal(fieldSummary({ name: 'click', fields: [{ name: 'a', type: 'string', required: true, max: 10 }] }).required, 1);
});

test('intake accepts a valid event and rejects an event its type refuses', () => {
  const { intake, store, dedupe } = build();
  const accepted = intake.accept('acme', [{ name: 'page_view', id: 'evt_1', properties: { path: '/docs' }, context: { user: 'usr_1' } }]);
  assert.equal(accepted.accepted, 1);
  assert.deepEqual(accepted.rejected, []);
  const refused = intake.accept('acme', [{ name: 'page_view', id: 'evt_2', properties: { duration_ms: 5 } }]);
  assert.equal(refused.accepted, 0);
  assert.deepEqual(refused.rejected[0].problems, ['path is required']);
  const duplicate = intake.accept('acme', [{ name: 'page_view', id: 'evt_1', properties: { path: '/docs' } }]);
  assert.deepEqual(duplicate.rejected[0].problems, ['duplicate event id']);
  assert.equal(dedupe.size(), 1);
  assert.equal(store.count('events'), 1);
  assert.equal(intake.counts('acme').total, 1);
  assert.equal(intake.eventsOf('acme').length, 1);
  assert.equal(intake.eventsOf('globex').length, 0);
});

test('a batch is planned from the samples its events carry', () => {
  const plan = { name: 'growth', limits: { max_rows: 5, max_samples: 10, max_bytes: 1024 } };
  const planned = planBatch(plan, { events: [{ name: 'page_view', samples: [1, 2] }, { name: 'page_view', samples: [3] }] });
  assert.equal(planned.accepted, 2);
  assert.equal(planned.samples, 3);
  assert.equal(planned.slots.length, 3);
  assert.throws(() => planBatch(plan, { events: Array.from({ length: 6 }, () => ({ name: 'page_view' })) }), /exceed the 5/);
  assert.throws(
    () => planBatch(plan, { events: [{ name: 'page_view', samples: Array.from({ length: 11 }, (_, index) => index) }] }),
    /exceed the 10/,
  );
  assert.equal(exceedsAny(plan, { rows: 3 }), undefined);
  assert.equal(exceedsAny(plan, { rows: 9 }), 'batch_too_large');
  assert.equal(limitReport(plan).samples, 10);
  assert.deepEqual(remainingCapacity(plan, { rows: 2, samples: 4, bytes: 1024 }).rows, 3);
  assert.throws(() => assertWithinLimits(plan, { rows: -1 }), /whole number/);
});

test('ndjson intake plans every line and refuses an oversized one', () => {
  const { intake } = build();
  const plans = new PlanBook();
  const ndjson = new NdjsonIntake({ intake, plans });
  const text = toNdjson([{ name: 'page_view', id: 'evt_10', properties: { path: '/a' } }, { name: 'page_view', id: 'evt_11', properties: { path: '/b' } }]);
  const result = ndjson.submit('acme', text);
  assert.equal(result.accepted, 2);
  assert.deepEqual(splitLines('\n a\n\nb \n'), [' a', 'b ']);
  assert.throws(() => parseNdjson('{ not json'), /line 1 is not valid JSON/);
  assert.throws(() => planNdjson({ name: 'free', limits: { max_line_bytes: 4 } }, '{"a":1}'), /exceed 4 bytes/);
  assert.equal(ndjson.summary('acme').rows, 2);
  assert.throws(() => ndjson.append('acme', 'x'.repeat(70 * 1024)), /at most 65536 bytes/);
});

test('a replay is planned, then applied or reported as a dry run', () => {
  const { intake, plans } = build();
  const replayer = new Replayer({ intake, plans, clock });
  const text = toNdjson([{ name: 'page_view', id: 'evt_20', properties: { path: '/a' }, timestamp: '2026-02-01T00:00:00.000Z' }]);
  assert.deepEqual(replayer.replay('acme', text, { dry_run: true }), { dry_run: true, rows: 1, samples: 0 });
  assert.equal(replayer.history('acme').length, 0);
  const run = replayer.replay('acme', text, { shift_ms: 3600000 });
  assert.equal(run.accepted, 1);
  assert.equal(replayer.history('acme').length, 1);
  assert.equal(replayer.stats().rows, 1);
  assert.throws(() => replayer.replayFile('acme', '  '), /at least one line/);
  assert.deepEqual(replayPlan([{ samples: [1, 2] }, {}], { chunk: 2 }), { rows: 2, samples: 2, bytes: Buffer.byteLength(JSON.stringify([{ samples: [1, 2] }, {}])), chunk: 2, batches: 1 });
  assert.equal(shiftTimestamps([{ timestamp: '2026-02-01T00:00:00.000Z' }], 60000)[0].timestamp, '2026-02-01T00:01:00.000Z');
});

test('an envelope carries its source onto every event when it is upgraded', () => {
  const envelope = normalizeEnvelope({ source: 'ios', events: [{ name: 'page_view', properties: { path: '/a' } }] });
  assert.equal(envelope.events[0].context.source, 'ios');
  assert.equal(envelopeSummary(envelope).events, 1);
  assert.equal(splitEnvelope(normalizeEnvelope({ events: Array.from({ length: 5 }, () => ({ name: 'page_view' })) }), 2).length, 3);
  const upgraded = upgradeEnvelope({ version: 1, source: 'web', events: [{ name: 'click' }] });
  assert.equal(upgraded.version, 2);
  assert.equal(upgraded.events[0].context.source, 'web');
  assert.equal(acceptedEnvelope({ events: [{ name: 'a' }, { name: 'b' }] }, 1).dropped, 1);
});

test('derived attributes are computed from the context, never from an input we trust', () => {
  const bot = enrichEvent({ name: 'page_view', context: { user_agent: 'Googlebot/2.1', country: 'de' } });
  assert.equal(bot.derived.bot, true);
  assert.equal(bot.derived.country, 'DE');
  const mobile = enrichEvent({ name: 'page_view', context: { user_agent: 'Mozilla iPhone' } });
  assert.equal(mobile.derived.device, 'mobile');
  assert.equal(humanShare([bot, mobile]), 50);
  assert.deepEqual(deviceBreakdown([bot, mobile]), { bot: 1, mobile: 1 });
  assert.equal(countryBreakdown([bot])[0].country, 'DE');
  assert.equal(referrerHost({ context: { referrer: 'https://news.example.com/a' } }), 'news.example.com');
  assert.equal(referrerHost({ context: {} }), null);
  assert.equal(botShare([bot, mobile]), 50);
});

test('samples are summarised without trusting the count a caller names', () => {
  const events = [{ name: 'page_view', samples: [1, 2, 3] }, { name: 'page_view', samples: [10] }];
  assert.equal(sampleTotals(events).values, 4);
  assert.equal(sampleTotals(events).sum, 16);
  assert.equal(histogram([1, 2, 3, 4], 2).count, 4);
  assert.equal(percentileSummary([1, 2, 3, 4]).p50, 3);
  const sketch = sampleSketch(events);
  assert.equal(sketch.max, 10);
  assert.equal(sketch.distinct, 4);
  assert.equal(truncateSamples({ samples: [1, 2, 3] }, 2).samples.length, 2);
});

test('a batch of properties is validated against the declared fields', () => {
  const type = { name: 'purchase', fields: [{ name: 'order_id', type: 'string', required: true }, { name: 'amount_cents', type: 'number', required: true }] };
  const report = validateBatch(type, [{ properties: { order_id: 'o1', amount_cents: 5 } }, { properties: { amount_cents: 5 } }]);
  assert.deepEqual(report.accepted, [0]);
  assert.equal(report.rejected[0].index, 1);
  assert.equal(report.rate, 50);
  assert.equal(validateEvent(type, { name: 'purchase', properties: { order_id: 'o1', amount_cents: 5 } }).ok, true);
});
