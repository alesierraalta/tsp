import test from 'node:test';
import assert from 'node:assert/strict';
import { rollup, rollupByEvent, densify, growthOf, peakOf, rollupTotals, spanHours } from '../src/rollups/rollup.js';
import { grouped, crossTab, timeSeries, rankedNames, QueryBook, describeSavedQuery, matchesFilters } from '../src/reports/queries.js';
import { defineFunnel, funnelCounts, funnelRates, describeFunnel } from '../src/reports/funnels.js';
import { firstSeen, cohortTable, cohortRetention, cohortReport } from '../src/reports/cohorts.js';
import { kpis, kpiTable, kpiSummary, kpiDeltas, topPaths } from '../src/reports/kpis.js';
import { toCsv, toNdjson, exportRows, rollupRows, reportRows, exportPlan, chunkedExport, exportSummary, manifestFor, isEmptyExport } from '../src/reports/exports.js';
import { toCsvCell, parseCsv, csvSummary, toCsvTable } from '../src/reports/csv.js';
import { aggregateSamples, aggregateTable, sampleQuantiles, perEventSamples } from '../src/rollups/aggregates.js';
import { compacted, compactPlan, mergeCompacted, compactionReport } from '../src/rollups/compaction.js';
import { forecast, trend, movingAverage, capacityRisk, stepsToLimit, growthRate } from '../src/rollups/forecast.js';

const event = (id, name, at, extra = {}) => ({
  id,
  tenant: 'acme',
  name,
  at,
  context: { user: extra.user ?? 'usr_1', path: extra.path ?? '/docs', session: extra.session ?? 'ses_1' },
  properties: extra.properties ?? {},
  samples: extra.samples,
});

const events = [
  event('evt_1', 'page_view', '2026-02-08T09:00:00.000Z', { user: 'usr_1', path: '/pricing', samples: [1, 2] }),
  event('evt_2', 'page_view', '2026-02-08T10:00:00.000Z', { user: 'usr_2', path: '/docs', samples: [3] }),
  event('evt_3', 'purchase', '2026-02-09T11:00:00.000Z', { user: 'usr_1', path: '/checkout', properties: { amount_cents: 500 } }),
  event('evt_4', 'page_view', '2026-02-10T09:00:00.000Z', { user: 'usr_3', path: '/pricing', samples: [4] }),
];

test('a rollup groups events into buckets and reports their shape', () => {
  const points = rollup(events, { size: 'day', metrics: ['samples'] });
  assert.equal(points.length, 3);
  assert.deepEqual(points[0], { bucket: '2026-02-08T00:00:00.000Z', events: 2, samples: 3 });
  assert.equal(rollupTotals(points).events, 4);
  assert.equal(growthOf(points), -50);
  assert.equal(peakOf(points).events, 2);
  assert.equal(spanHours(points), 48);
  assert.equal(rollupByEvent(events, 'day').length, 3);
  assert.deepEqual(densify(points, '2026-02-08', '2026-02-10', 'day').length, 3);
});

test('a grouping is what the report renders, with the filters the caller may narrow by', () => {
  assert.deepEqual(grouped(events, 'name').map((group) => [group.events, group.users]), [[3, 3], [1, 1]]);
  assert.equal(crossTab(events, 'name', 'day').length, 2);
  assert.equal(timeSeries(events, 'day').length, 3);
  assert.deepEqual(rankedNames(events)[0], { name: 'page_view', count: 3 });
  assert.equal(matchesFilters(events[0], [{ path: 'name', value: 'page_view' }]), true);
  assert.equal(matchesFilters(events[0], [{ path: 'context.user', op: 'ne', value: 'usr_1' }]), false);
});

test('a saved query runs its grouping over the filtered events', () => {
  const book = new QueryBook();
  book.save({ name: 'Daily views', group: 'day' });
  assert.deepEqual(describeSavedQuery(book.get('daily-views')), { name: 'daily-views', grouping: 'day', minimum: 0, limit: 50, filters: 0 });
  const result = book.run('daily-views', events);
  assert.equal(result.events, 4);
  assert.equal(result.rows.length, 3);
  assert.equal(book.list().length, 1);
  assert.equal(book.stats().byGrouping.day, 1);
  assert.equal(book.remove('daily-views'), true);
  assert.throws(() => book.run('daily-views', events), /no saved query/);
});

test('a funnel counts the steps in order and reports its dropoff', () => {
  const funnel = defineFunnel(['page_view', 'purchase']);
  const counts = funnelCounts(events, funnel);
  assert.equal(counts.length, 2);
  assert.equal(funnelRates(counts)[0].percent, 100);
  assert.equal(describeFunnel(events, funnel).steps, 2);
  assert.throws(() => defineFunnel(['page_view']), /at least two steps/);
});

test('a cohort starts at the day a user was first seen', () => {
  const first = firstSeen(events);
  assert.equal(first.get('usr_1'), '2026-02-08');
  assert.equal(cohortTable(events).length, 2);
  const retention = cohortRetention(events, 3);
  assert.equal(retention.length, 3);
  assert.equal(cohortReport(events, 2).users, 3);
});

test('the kpis count users and samples without double counting them', () => {
  const counts = kpis(events);
  assert.equal(counts.events, 4);
  assert.equal(counts.users, 3);
  assert.equal(counts.samples, 4);
  assert.equal(kpiTable(events).length, 4);
  assert.equal(kpiSummary(events).headline.name, 'events');
  assert.equal(kpiDeltas({ events: 2 }, { events: 4 }).events.percent, 100);
  assert.deepEqual(topPaths(events)[0], { path: '/pricing', events: 2 });
});

test('an export describes what it wrote before it is written', () => {
  const rows = [{ bucket: '2026-02-08', events: 2 }, { bucket: '2026-02-09', events: 1 }];
  assert.equal(toCsv(rows), 'bucket,events\n2026-02-08,2\n2026-02-09,1\n');
  assert.equal(toNdjson(rows).trim().split('\n').length, 2);
  assert.equal(exportRows(rows, 'json').includes('"rows"'), true);
  assert.throws(() => exportRows(rows, 'xml'), /unknown export format/);
  assert.equal(rollupRows([{ bucket: 'a', events: 2, samples: 1 }])[0].samples, 1);
  assert.equal(reportRows('acme', [{ bucket: 'a', events: 2 }], 2)[0].share, 100);
  assert.equal(exportPlan([1, 2, 3], 2).files, 2);
  const files = chunkedExport('rollup', rows, 'csv', 1);
  assert.deepEqual(files.map((file) => file.name), ['rollup-001.csv', 'rollup-002.csv']);
  assert.equal(exportSummary(files).rows, 2);
  assert.equal(manifestFor('rollup', rows, 'csv', { now: new Date('2026-02-10T12:00:00.000Z') }).generated_at, '2026-02-10T12:00:00.000Z');
  assert.equal(isEmptyExport([]), true);
});

test('csv cells quote what would otherwise split a row', () => {
  assert.equal(toCsvCell('a,b'), '"a,b"');
  assert.equal(toCsvCell('say "hi"'), '"say ""hi"""');
  assert.deepEqual(parseCsv('a,b\n1,2\n'), { fields: ['a', 'b'], rows: [{ a: '1', b: '2' }] });
  assert.deepEqual(csvSummary('a,b\n1,2\n'), { fields: 2, rows: 1 });
  assert.equal(toCsvTable([{ a: 1 }]), 'a\n1\n');
});

test('sample aggregates answer the questions the dashboards ask', () => {
  const aggregate = aggregateSamples(events);
  assert.equal(aggregate.count, 4);
  assert.equal(aggregate.sum, 10);
  assert.equal(aggregateTable(events).length, 2);
  assert.equal(sampleQuantiles(events, [0.5])[0].value, 3);
  assert.equal(perEventSamples(events).filter((entry) => entry.samples > 0).length, 3);
});

test('compaction folds hour points into wider buckets', () => {
  const points = [
    { bucket: 'a', events: 1, samples: 1 },
    { bucket: 'b', events: 2, samples: 2 },
    { bucket: 'c', events: 3, samples: 3 },
  ];
  const folded = compacted(points, 2);
  assert.equal(folded.length, 2);
  assert.equal(folded[0].events, 3);
  assert.equal(compactionReport(points, folded).saved, 1);
  assert.equal(compactPlan().length, 3);
  assert.equal(mergeCompacted([{ bucket: 'a', events: 1 }], [{ bucket: 'a', events: 2 }])[0].events, 3);
});

test('a forecast moves at the slope the series has shown', () => {
  const values = [1, 2, 3, 4];
  assert.equal(trend(values), 'up');
  assert.equal(movingAverage(values, 2)[3], 4);
  assert.equal(growthRate(values), 300);
  assert.equal(forecast([{ events: 1 }, { events: 2 }], 1)[0].value, 3);
  assert.equal(stepsToLimit(values, 100) > 0, true);
  assert.equal(capacityRisk([1, 2, 3], 100).risk, 'low');
  assert.equal(capacityRisk([1, 2, 3], 20).risk, 'medium');
  assert.equal(capacityRisk([1, 2, 3], 5).risk, 'high');
});
