import test from 'node:test';
import assert from 'node:assert/strict';
import { ENVIRONMENTS, environmentOf, normalizeEnvironment, isProtected, approvalsRequired, rolloutStepsOf, environmentTable, higherOf } from '../src/domain/environments.js';
import { ServiceCatalog, normalizeService, catalogReport } from '../src/domain/services.js';
import { parseVersion, compareVersions, bumpVersion, isPrerelease, channelOf, sortVersions, latestOf, nextVersion } from '../src/domain/versions.js';
import { ReleaseBook, normalizeRelease, canTransition, TRANSITIONS, RELEASE_STATUSES, summarizeReleases, releaseWindows } from '../src/domain/releases.js';
import { normalizeChange, normalizeChanges, groupChanges, breakingChanges, changeSummary, mergeChanges } from '../src/domain/changes.js';
import { FreezeCalendar, normalizeFreeze, covers, durationMinutes, freezeReport } from '../src/domain/freeze.js';
import { PolicyBook, normalizePolicy, evaluate, describePolicy, policyRows } from '../src/domain/policies.js';
import { clock, releaseInput } from './helpers.js';

test('an environment carries its tier, approvals and rollout steps', () => {
  assert.deepEqual(ENVIRONMENTS, ['development', 'staging', 'production']);
  assert.equal(environmentOf('production'), 'production');
  assert.equal(environmentOf('mars'), 'development');
  assert.equal(isProtected('production'), true);
  assert.equal(isProtected('staging'), false);
  assert.equal(approvalsRequired('production'), 2);
  assert.deepEqual(rolloutStepsOf('staging'), [50, 100]);
  const production = normalizeEnvironment({ name: 'production' });
  assert.equal(production.display, 'Production');
  assert.equal(production.protected, true);
  assert.throws(() => normalizeEnvironment({ name: 'mars' }), /is not known/);
  assert.equal(environmentTable().length, 3);
  assert.equal(higherOf('staging', 'production'), 'production');
});

test('a service is registered once with its owner and language', () => {
  const catalog = new ServiceCatalog({ clock });
  const service = catalog.define({ name: 'Checkout API', owner: 'payments', language: 'node' });
  assert.equal(service.id, 'checkout-api');
  assert.throws(() => catalog.define({ name: 'Checkout API' }), /already exists/);
  assert.equal(catalog.require('checkout-api').owner, 'payments');
  catalog.define({ name: 'Search', owner: 'discovery', language: 'go' });
  assert.equal(catalog.list().length, 2);
  assert.equal(catalog.list('go').length, 1);
  assert.deepEqual(catalog.byOwner(), { payments: 1, discovery: 1 });
  assert.equal(catalog.stats().services, 2);
  assert.equal(catalogReport(catalog).owners, 2);
  assert.equal(catalog.update('checkout-api', { owner: 'core' }).owner, 'core');
  assert.equal(catalog.remove('search'), true);
  assert.throws(() => normalizeService({ name: '' }), /needs a name/);
});

test('versions compare, bump and name their channel', () => {
  assert.deepEqual(parseVersion('1.4.2'), { major: 1, minor: 4, patch: 2, prerelease: null });
  assert.equal(compareVersions('1.4.2', '1.4.10') < 0, true);
  assert.equal(compareVersions('1.4.0-rc.1', '1.4.0'), -1);
  assert.equal(bumpVersion('1.4.2', 'minor'), '1.5.0');
  assert.equal(bumpVersion('1.4.2', 'major'), '2.0.0');
  assert.equal(bumpVersion('1.4.2', 'patch'), '1.4.3');
  assert.equal(isPrerelease('1.4.0-rc.1'), true);
  assert.equal(channelOf('1.4.0-rc.1'), 'candidate');
  assert.equal(channelOf('1.4.0'), 'stable');
  assert.equal(latestOf(['1.4.0', '1.10.0', '1.9.9']), '1.10.0');
  assert.deepEqual(sortVersions(['1.4.0', '1.10.0']), ['1.10.0', '1.4.0']);
  assert.equal(nextVersion(['1.4.0'], 'minor'), '1.5.0');
  assert.throws(() => parseVersion('1.4'), /not a semantic version/);
});

test('a release is created once per service, version and environment', () => {
  const releases = new ReleaseBook({ clock });
  const release = releases.create({ ...releaseInput(), tenant: 'acme', created_by: 'usr_1' });
  assert.equal(release.status, 'planned');
  assert.equal(release.channel, 'stable');
  assert.equal(release.major, 1);
  assert.equal(releases.create({ ...releaseInput({ version: '1.4.1' }), tenant: 'acme' }).version, '1.4.1');
  assert.throws(() => releases.create({ ...releaseInput(), tenant: 'acme' }), /already exists/);
  assert.equal(releases.get(release.id).service, 'checkout');
  assert.equal(releases.list({ tenant: 'acme' }).length, 2);
  assert.equal(releases.list({ environment: 'staging' }).length, 0);
  assert.equal(releases.list({ channel: 'stable' }).length, 2);
  assert.equal(releases.stats().releases, 2);
  assert.deepEqual(releases.stats().byStatus, { planned: 2 });
  assert.throws(() => normalizeRelease({ version: 'nope', service: 'checkout' }), /not a semantic version/);
  assert.throws(() => normalizeRelease({ version: '1.0.0', service: '' }), /names the service/);
});

test('a release only moves along the transitions the table allows', () => {
  const releases = new ReleaseBook({ clock });
  const release = releases.create({ ...releaseInput(), tenant: 'acme' });
  assert.deepEqual(TRANSITIONS.planned, ['staged', 'cancelled']);
  assert.equal(canTransition('planned', 'live'), false);
  assert.equal(releases.transition(release.id, 'staged').status, 'staged');
  assert.throws(() => releases.transition(release.id, 'planned'), /cannot go from staged to planned/);
  assert.equal(releases.transition(release.id, 'canary').status, 'canary');
  assert.equal(releases.transition(release.id, 'live').status, 'live');
  assert.throws(() => releases.cancel(release.id), /cannot go from live to cancelled/);
  assert.equal(RELEASE_STATUSES.includes('rolled_back'), true);
  assert.equal(releases.promote(release.id).environment, 'staging');
  assert.equal(releases.promote(release.id).environment, 'production');
  assert.throws(() => releases.promote(release.id), /last environment/);
});

test('release windows and summaries describe what shipped', () => {
  const releases = new ReleaseBook({ clock });
  const first = releases.create({ ...releaseInput({ service: 'checkout' }), tenant: 'acme' });
  releases.create({ ...releaseInput({ service: 'checkout', version: '1.4.1' }), tenant: 'acme' });
  releases.create({ ...releaseInput({ service: 'search', version: '2.0.0' }), tenant: 'acme' });
  const windows = releaseWindows(releases.list({ tenant: 'acme' }));
  assert.equal(windows.length, 2);
  assert.equal(windows.find((window) => window.service === 'checkout').releases, 2);
  const summary = summarizeReleases(releases.list({ tenant: 'acme' }));
  assert.equal(summary.releases, 3);
  assert.equal(summary.services, 2);
  assert.equal(summary.planned, 3);
  assert.equal(releases.liveFor('production').length, 0);
  assert.equal(releases.protectedOnes().length, 0);
  assert.equal(releases.get(first.id).id, first.id);
});

test('changes are grouped the way the notes render them', () => {
  const changes = normalizeChanges([
    { kind: 'feature', summary: 'Add card selector' },
    { kind: 'fix', summary: 'Stop double charging' },
    { kind: 'breaking', summary: 'Drop v1 webhook shape' },
  ]);
  assert.equal(changes.length, 3);
  assert.equal(normalizeChange({ summary: 'plain string' }).kind, 'chore');
  assert.equal(groupChanges(changes).feature.length, 1);
  assert.equal(breakingChanges(changes).length, 1);
  assert.deepEqual(changeSummary(changes), { changes: 3, byKind: { feature: 1, fix: 1, breaking: 1 }, breaking: 1, security: 0 });
  assert.equal(mergeChanges(changes, changes).length, 3);
  assert.throws(() => normalizeChange({ summary: '' }), /needs a summary/);
});

test('a freeze window covers the instants between its edges and blocks a promotion', () => {
  const calendar = new FreezeCalendar({ clock });
  const freeze = calendar.add({ environment: 'production', from: '2026-02-10T00:00:00.000Z', to: '2026-02-12T00:00:00.000Z', reason: 'audit' });
  assert.equal(covers(freeze, '2026-02-11T00:00:00.000Z'), true);
  assert.equal(covers(freeze, '2026-02-13T00:00:00.000Z'), false);
  assert.equal(durationMinutes(freeze), 2880);
  assert.equal(calendar.blocks('production', '2026-02-11T00:00:00.000Z'), true);
  assert.equal(calendar.blocks('staging', '2026-02-11T00:00:00.000Z'), false);
  assert.equal(calendar.stats().active, true);
  assert.equal(freezeReport(calendar, 'production').minutes, 2880);
  assert.equal(calendar.remove(freeze.id), true);
  assert.equal(calendar.stats().freezes, 0);
  assert.throws(() => normalizeFreeze({ from: '2026-02-10T00:00:00.000Z', to: '2026-02-09T00:00:00.000Z' }), /ends after it starts/);
});

test('a policy answers whether a role may deploy where', () => {
  const book = new PolicyBook({});
  assert.equal(book.check('deploy', 'development', { role: 'releaser' }).allowed, true);
  const refused = book.check('deploy', 'production', { role: 'releaser' });
  assert.equal(refused.allowed, false);
  assert.match(refused.reasons[0], /below approver/);
  assert.equal(book.check('deploy', 'production', { role: 'owner' }).allowed, true);
  assert.equal(book.check('deploy', 'production', { role: 'approver', approvals: 2 }).allowed, true);
  assert.equal(book.check('deploy', 'production', { role: 'approver', approvals: 1 }).allowed, false);
  const custom = book.define({ kind: 'deploy', environment: 'staging', minimum_role: 'approver', allowed_channels: ['candidate'] });
  assert.equal(book.check('deploy', 'staging', { role: 'approver', channel: 'stable' }).allowed, false);
  assert.equal(describePolicy(custom).minimum_role, 'approver');
  assert.equal(policyRows(book).length, 4);
  assert.equal(book.matrix(['development', 'production']).length, 2);
  assert.equal(evaluate(normalizePolicy({ kind: 'deploy', environment: 'production' }), { role: 'owner', approvals: 2 }).allowed, true);
  assert.equal(book.stats().byKind.deploy, 3);
});
