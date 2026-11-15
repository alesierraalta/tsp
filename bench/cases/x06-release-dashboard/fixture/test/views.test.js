import test from 'node:test';
import assert from 'node:assert/strict';
import { escapeText, escapeAttr, attrPair, element, rawJson, scriptJson, classNames, stripControl } from '../src/views/escaping.js';
import { renderServiceTag, renderOwnerTag, renderReleaseTag, renderEnvironmentTag, renderStatusPill, renderSeverityBadge, renderChangelog, renderKeyValue, renderHostList, renderDeliveryState, renderPolicyList } from '../src/views/partials.js';
import { button, link, card, stat, statRow, avatar, progress, sparkline } from '../src/views/components.js';
import { table, releaseColumns, deploymentColumns, emptyTable, sortableHeader } from '../src/views/tables.js';
import { page, navigation, head, footer, section, twoColumn, empty, notice } from '../src/views/layout.js';
import { dashboardPage, dashboardStats, activityRows, statusStrip, servicePanel } from '../src/views/dashboard.js';
import { releasePage, releasesPage, releaseHeader } from '../src/views/releases.js';
import { deploymentPage, stageList, approvalTrail, deploymentsPage } from '../src/views/deployments.js';
import { webhooksPage, subscriptionsPanel, retryNotice } from '../src/views/webhooks.js';
import { incidentsPage, incidentPage, severityGuide, blockingPanel } from '../src/views/incidents.js';
import { readinessPanel, readinessSummary, checkRows, canPromote, blockedGates, readinessBanner } from '../src/views/readiness.js';
import { seedDeployment, appWithKeys, TENANT } from './helpers.js';

const QUOTED = 'checkout" onmouseover="alert(1)';

test('the two escapers escape what their own context needs', () => {
  assert.equal(escapeText('<b>&'), '&lt;b&gt;&amp;');
  assert.equal(escapeText('a "b"'), 'a "b"');
  assert.equal(escapeAttr('<b>&'), '&lt;b&gt;&amp;');
  assert.equal(escapeAttr('a "b"'), 'a &quot;b&quot;');
  assert.equal(escapeAttr("a 'b'"), 'a &#39;b&#39;');
  assert.equal(attrPair('data-env', QUOTED), 'data-env="checkout&quot; onmouseover=&quot;alert(1)"');
  assert.throws(() => attrPair('data env', 'x'), /invalid attribute name/);
  assert.equal(element('p', { class: 'note' }, escapeText('<hi>')), '<p class="note">&lt;hi&gt;</p>');
  assert.equal(classNames('a', ['b'], null), 'a b');
  assert.equal(stripControl('a\u0000b'), 'ab');
  assert.equal(rawJson({ a: '</script>' }).includes('\\u003c'), true);
  assert.equal(scriptJson({ a: 1 }).startsWith('<script type="application/json">'), true);
});

test('a partial writes a stored name into its attribute through the attribute escaper', () => {
  const tag = renderServiceTag({ name: QUOTED });
  assert.equal(tag.includes('data-service="checkout&quot; onmouseover=&quot;alert(1)"'), true);
  assert.equal(tag.includes('onmouseover="alert(1)"'), false);
  assert.equal(renderOwnerTag(QUOTED).includes('&quot;'), true);
  assert.equal(renderReleaseTag({ version: '1.4.0', channel: 'stable' }).includes('data-version="1.4.0"'), true);
  assert.equal(renderStatusPill('awaiting_approval').includes('Awaiting Approval'), true);
  assert.equal(renderSeverityBadge('sev1').includes('data-severity="sev1"'), true);
  assert.equal(renderDeliveryState('posted').includes('posted'), true);
  assert.equal(renderPolicyList([]).includes('No policies'), true);
  assert.equal(renderKeyValue({ service: 'checkout' }).includes('data-field="service"'), true);
});

test('a partial writes an environment label into its attribute and into its text', () => {
  const tag = renderEnvironmentTag({ name: 'production', label: 'production (eu)' });
  assert.equal(tag.includes('data-environment="production (eu)"'), true);
  assert.equal(tag.includes('>Production (eu)<'), true);
});

test('the partials that write a stored name into an attribute escape the quote', () => {
  const rendered = [
    renderServiceTag({ name: QUOTED }),
    renderOwnerTag(QUOTED),
    renderReleaseTag({ version: '1.0.0', channel: QUOTED }),
    renderStatusPill(QUOTED),
    renderSeverityBadge(QUOTED),
  ].join('');
  assert.equal(rendered.includes(`="${QUOTED}"`), false);
  assert.equal(rendered.split('&quot;').length - 1 >= 8, true);
  assert.equal(attrPair('data-owner', QUOTED).includes('&quot;'), true);
});

test('the changelog, host list and cards escape their text', () => {
  const changelog = renderChangelog([{ kind: 'feature', summary: '<script>x</script>' }]);
  assert.equal(changelog.includes('&lt;script&gt;'), true);
  assert.equal(changelog.includes('<script>'), false);
  assert.equal(renderChangelog([]).includes('No changes'), true);
  assert.equal(renderHostList({ 'hooks.example.com': 2 }).includes('2 deliveries'), true);
  assert.equal(renderHostList({}).includes('No deliveries'), true);
  assert.equal(card('Title', '<p>body</p>').includes('<h2>Title</h2>'), true);
  assert.equal(button('Publish', { variant: 'primary' }).includes('class="button primary"'), true);
  assert.equal(link('details', '/releases/1').includes('href="/releases/1"'), true);
  assert.equal(avatar('Ada Lovelace').includes('AL'), true);
  assert.equal(stat('releases', 3).includes('data-stat="releases"'), true);
  assert.equal(statRow([{ label: 'a', value: 1 }]).includes('class="stats"'), true);
  assert.equal(progress(1, 4).includes('25%'), true);
  assert.equal(sparkline([1, 2]).includes('sparkline'), true);
});

test('a page carries its navigation, its title and its sections', () => {
  const rendered = page({ title: 'Releases', section: 'releases', tenant: 'acme', body: section('list', '<p>rows</p>') });
  assert.equal(rendered.startsWith('<!doctype html>'), true);
  assert.equal(rendered.includes('<title>Releases</title>'), true);
  assert.equal(rendered.includes('nav-link active'), true);
  assert.equal(rendered.includes('class="panel list"'), true);
  assert.equal(rendered.includes('acme'), true);
  assert.equal(navigation('dashboard').includes('href="/"'), true);
  assert.equal(head('x').includes('<meta charset="utf-8">'), true);
  assert.equal(footer({ version: '2.0.0' }).includes('2.0.0'), true);
  assert.equal(twoColumn('<p>l</p>', '<p>r</p>').includes('class="columns"'), true);
  assert.equal(empty('nothing').includes('class="empty"'), true);
  assert.equal(notice('careful', 'warn').includes('notice warn'), true);
});

test('a table renders one cell per column and escapes its text', () => {
  const rows = [{ version: '1.0.0', service: '<bad>', environment: 'production', status: 'live', created_at: '2026-02-09T10:00:00.000Z' }];
  const rendered = table(releaseColumns(), rows);
  assert.equal(rendered.includes('<thead>'), true);
  assert.equal(rendered.includes('&lt;bad&gt;'), true);
  assert.equal(rendered.includes('data-column="version"'), true);
  assert.equal(deploymentColumns().length, 6);
  assert.equal(emptyTable('nothing').includes('class="empty"'), true);
  assert.equal(sortableHeader('Version', 'version', { direction: 'asc' }).includes('direction=desc'), true);
});

test('the dashboard page renders what the view was handed', () => {
  const { app } = appWithKeys();
  const { release, deployment } = seedDeployment(app);
  const view = {
    title: 'Release dashboard',
    tenant: TENANT,
    environments: ['development', 'staging', 'production'],
    environmentLabels: { production: 'Production (EU)' },
    environment: 'production',
    summary: dashboardStats([deployment], [release], []),
    byEnvironment: { development: 1 },
    deployments: [deployment],
    incidents: [],
    current: deployment,
    deliveries: [],
    hosts: {},
  };
  const rendered = dashboardPage(view);
  assert.equal(rendered.includes('Production (EU)'), true);
  assert.equal(rendered.includes('data-environment="Production (EU)"'), true);
  assert.equal(rendered.includes('checkout'), true);
  assert.equal(dashboardStats([deployment], [release], []).failure_rate, 0);
  assert.equal(activityRows([deployment]).length, 1);
  assert.equal(statusStrip({ live: 2 }).includes('Live'), true);
  assert.equal(servicePanel([]).includes('Services'), true);
});

test('the release, deployment, webhook and incident pages render their records', () => {
  const { app } = appWithKeys();
  const { release, deployment } = seedDeployment(app);
  assert.equal(releasesPage([release]).includes('1.4.0'), true);
  assert.equal(releasePage(release, { deployments: [deployment] }).includes('checkout'), true);
  assert.equal(releaseHeader(release).includes('data-version="1.4.0"'), true);
  assert.equal(deploymentsPage([deployment]).includes('deployments'), true);
  assert.equal(deploymentPage(deployment).includes('data-stage="prepare"'), true);
  assert.equal(stageList(deployment).includes('current'), true);
  assert.equal(approvalTrail([]).includes('No approvals'), true);
  assert.equal(webhooksPage({ subscriptions: [], deliveries: [], stats: { deliveries: 0 }, hosts: {} }).includes('Webhooks'), true);
  assert.equal(subscriptionsPanel([]).includes('No subscriptions'), true);
  assert.equal(retryNotice({ attempts: 4, baseSeconds: 15, maxSeconds: 900 }).includes('4 attempts'), true);
  assert.equal(incidentsPage([]).includes('Incidents'), true);
  assert.equal(severityGuide().includes('sev1'), true);
  assert.equal(blockingPanel([]).includes('Nothing blocks'), true);
  assert.equal(
    incidentPage({ id: 'inc_1', title: 'Down', severity: 'sev1', service: 'checkout', environment: 'production', state: 'open', opened_at: '2026-02-10T10:00:00.000Z', tenant: TENANT }).includes('Down'),
    true,
  );
});

test('the readiness panel grades the gates a promotion passes', () => {
  const view = { approvals: 2, required: 2, blocking: 0, freeze: { active: false }, policy: { allowed: true, reasons: [] }, steps: 4 };
  assert.equal(readinessSummary(view).state, 'go');
  assert.equal(canPromote(view), true);
  assert.equal(blockedGates(view).length, 0);
  assert.equal(checkRows(view).length, 5);
  assert.equal(readinessSummary({ ...view, blocking: 1 }).state, 'hold');
  assert.equal(blockedGates({ ...view, blocking: 1 }).includes('incidents'), true);
  assert.equal(readinessSummary({ ...view, approvals: 0 }).state, 'careful');
  assert.equal(readinessPanel(view).includes('Readiness: go'), true);
  assert.equal(readinessBanner('hold', 'held').includes('data-readiness="hold"'), true);
});
