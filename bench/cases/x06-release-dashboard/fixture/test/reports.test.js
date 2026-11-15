import test from 'node:test';
import assert from 'node:assert/strict';
import { releaseSummary, deploymentSummary, combinedSummary, summaryByEnvironment, summaryByService, busiestService, needsAttention } from '../src/reports/summary.js';
import { buildTimeline, timelineRows, groupByDay, countKinds, timelineSpan, describeTimeline, unmatchedKinds } from '../src/reports/timeline.js';
import { leadTime, deployFrequency, changeFailureRate, mttr, metricTable, doraTable, bandOf, failureRateByEnvironment, windowedMetrics } from '../src/reports/metrics.js';
import { cadenceByWeek, cadenceByService, cadenceReport, releaseInterval, churn, busiestWeek } from '../src/reports/cadence.js';
import { releaseNotes, notesMarkdown, notesText, notesSummary, breakingNotices, mergeNotes } from '../src/reports/notes.js';
import { toCsv, toNdjson, renderRows, chunkedExport, manifestFor, exportSummary, releaseRows, deploymentRows, incidentRows, isEmptyExport } from '../src/reports/export.js';
import { toCsvCell, parseCsv, csvSummary } from '../src/reports/csv.js';
import { serviceHealth, healthTable, overallHealth, healthReport, dailyFailures, healthyShare } from '../src/reports/readiness.js';
import { clock, releaseInput } from './helpers.js';
import { ReleaseBook } from '../src/domain/releases.js';
import { DeploymentBook } from '../src/domain/deployments.js';
import { IncidentBook } from '../src/domain/incidents.js';

function fixture() {
  const releases = new ReleaseBook({ clock });
  const deployments = new DeploymentBook({ clock });
  const incidents = new IncidentBook({ clock });
  const first = releases.create({ ...releaseInput(), tenant: 'acme' });
  const second = releases.create({ ...releaseInput({ version: '1.4.1', service: 'search' }), tenant: 'acme' });
  const deployed = deployments.start(first, { actor: 'usr_1' });
  deployments.finish(deployed.id, 'succeeded');
  return { releases, deployments, incidents, first, second, deployed };
}

test('a release and a deployment summary count what happened', () => {
  const { releases, deployments, first, second } = fixture();
  const summary = releaseSummary(releases.list({ tenant: 'acme' }));
  assert.equal(summary.releases, 2);
  assert.equal(summary.services, 2);
  assert.deepEqual(summary.byEnvironment, { development: 2 });
  assert.equal(busiestService(releases.list({ tenant: 'acme' })).releases, 1);
  const deploymentsOfTenant = deployments.list({ tenant: 'acme' });
  const deployment = deploymentSummary(deploymentsOfTenant);
  assert.equal(deployment.deployments, 1);
  assert.equal(deployment.succeeded, 1);
  assert.equal(deployment.success_rate, 100);
  assert.equal(summaryByEnvironment(deploymentsOfTenant)[0].environment, 'development');
  assert.equal(summaryByService(releases.list(), deploymentsOfTenant).length, 2);
  assert.deepEqual(combinedSummary(releases.list(), deploymentsOfTenant).releases.releases, 2);
  assert.equal(needsAttention(deploymentsOfTenant, []).failed, 0);
  assert.equal(first.id !== second.id, true);
});

test('a timeline mixes releases, deployments and incidents in one order', () => {
  const { releases, deployments, incidents } = fixture();
  incidents.open({ title: 'Latency', severity: 'sev2', service: 'checkout' });
  const sources = { releases: releases.list(), deployments: deployments.list(), incidents: incidents.list(), approvals: [] };
  const entries = buildTimeline(sources);
  assert.equal(entries.length, 5);
  assert.equal(countKinds(entries).deployment, 2);
  assert.equal(countKinds(entries).release, 2);
  assert.equal(timelineRows(entries).length, 5);
  assert.equal(groupByDay(entries).length, 1);
  assert.equal(timelineSpan(entries).minutes, 0);
  assert.equal(describeTimeline(entries).entries, 5);
  assert.deepEqual(unmatchedKinds(entries), ['rollback', 'approval']);
});

test('the dora metrics place the releases in a band', () => {
  const { deployments, incidents } = fixture();
  const finished = deployments.list();
  assert.equal(leadTime(finished).count, 1);
  assert.equal(deployFrequency(finished).per_week, 1);
  assert.deepEqual(changeFailureRate(finished), { finished: 1, failed: 0, rate: 0 });
  assert.equal(mttr([]).incidents, 0);
  assert.equal(metricTable(finished, incidents.list()).length, 4);
  assert.equal(doraTable(finished, incidents.list()).length, 4);
  assert.equal(bandOf(5, [10, 20, 30]), 'elite');
  assert.equal(bandOf(25, [10, 20, 30]), 'medium');
  assert.equal(failureRateByEnvironment(finished).length, 1);
  assert.equal(windowedMetrics(finished, '2026-02-01T00:00:00.000Z', '2026-02-28T00:00:00.000Z').deployments, 1);
});

test('a cadence report counts releases per week and per service', () => {
  const { releases } = fixture();
  const list = releases.list();
  assert.equal(cadenceByWeek(list).length, 1);
  assert.equal(cadenceByService(list).length, 2);
  assert.equal(cadenceReport(list).releases, 2);
  assert.equal(cadenceReport(list).per_week, 2);
  assert.equal(churn(list).length, 2);
  assert.equal(busiestWeek(list).releases, 2);
  assert.equal(releaseInterval(list, 'checkout').releases, 1);
});

test('release notes group the changelog into sections', () => {
  const { first } = fixture();
  const notes = releaseNotes({ ...first, changelog: [{ kind: 'feature', summary: 'Add card selector' }, { kind: 'breaking', summary: 'Drop v1 shape' }] });
  assert.equal(notes.sections.length, 2);
  assert.equal(notes.breaking, true);
  assert.equal(notesSummary(notes).changes, 2);
  assert.deepEqual(breakingNotices(notes), ['Drop v1 shape']);
  assert.equal(notesMarkdown(notes).includes('## Breaking'), true);
  assert.equal(notesText(notes).includes('FEATURE'), true);
  assert.equal(mergeNotes([notes, notes]).length, 2);
});

test('an export describes what it wrote before it is written', () => {
  const rows = [{ id: 'rel_1', version: '1.0.0' }];
  assert.equal(toCsv(rows), 'id,version\nrel_1,1.0.0\n');
  assert.equal(toNdjson(rows).trim().split('\n').length, 1);
  assert.equal(renderRows(rows, 'json').includes('"rows"'), true);
  assert.throws(() => renderRows(rows, 'xml'), /unknown export format/);
  assert.deepEqual(parseCsv(toCsv(rows)), { fields: ['id', 'version'], rows: [{ id: 'rel_1', version: '1.0.0' }] });
  assert.equal(csvSummary(toCsv(rows)).rows, 1);
  assert.equal(toCsvCell('a,b'), '"a,b"');
  const files = chunkedExport('releases', rows, 'csv', 1);
  assert.equal(files.length, 1);
  assert.equal(exportSummary(files).rows, 1);
  const manifest = manifestFor('releases', rows, 'csv', { now: new Date('2026-02-10T12:00:00.000Z') });
  assert.equal(manifest.generated_at, '2026-02-10T12:00:00.000Z');
  assert.equal(manifest.columns.length, 2);
  assert.equal(isEmptyExport([]), true);
});

test('the export rows are the columns a reader expects', () => {
  const { releases, deployments, incidents } = fixture();
  const incident = incidents.open({ title: 'Latency', severity: 'sev2', service: 'checkout' });
  assert.deepEqual(Object.keys(releaseRows(releases.list())[0]).includes('version'), true);
  assert.deepEqual(Object.keys(deploymentRows(deployments.list())[0]).includes('stage'), true);
  assert.deepEqual(Object.keys(incidentRows([incident])[0]).includes('severity'), true);
});

test('a health report grades a service from its incidents and deployments', () => {
  const { deployments, incidents } = fixture();
  const healthy = serviceHealth('checkout', deployments.list(), incidents.list());
  assert.equal(healthy.state, 'healthy');
  assert.equal(healthy.success_rate, 100);
  incidents.open({ title: 'Down', severity: 'sev1', service: 'checkout' });
  const degraded = serviceHealth('checkout', deployments.list(), incidents.list());
  assert.equal(degraded.state, 'degraded');
  assert.equal(degraded.blocking, 1);
  const rows = healthTable(['checkout', 'search'], deployments.list(), incidents.list());
  assert.equal(overallHealth(rows), 'degraded');
  assert.equal(healthReport(['checkout'], deployments.list(), incidents.list()).degraded, 1);
  assert.equal(dailyFailures(deployments.list()).length, 0);
  assert.equal(healthyShare([{ state: 'healthy' }]), 100);
});
