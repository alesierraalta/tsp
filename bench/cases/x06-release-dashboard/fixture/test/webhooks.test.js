import test from 'node:test';
import assert from 'node:assert/strict';
import { assertTarget, parseTarget, isInternalHost, hostAllowed, policyReport, hostOf, describeDecision, ALLOWED_HOSTS } from '../src/webhooks/targets.js';
import { SubscriptionBook, normalizeSubscription, subscriptionSummary, WEBHOOK_EVENTS } from '../src/webhooks/subscriptions.js';
import { signDelivery, verifySignature, parseSignature, signatureOf, SecretBook, signatureAge } from '../src/webhooks/signature.js';
import { RetryTracker, normalizePolicy, delaySeconds, attemptsFor, nextAttemptAt, isExhausted, describePolicy, scheduleTable } from '../src/webhooks/retry.js';
import { DeliveryLog, deliverySummary, successByHost, attemptHistory, retryCandidates, deliveryReport } from '../src/webhooks/delivery.js';
import { WebhookSender, recordingTransport } from '../src/webhooks/sender.js';
import { buildPayload, releasePayload, deploymentPayload, payloadEvents, describePayload } from '../src/webhooks/payloads.js';
import { clock, releaseInput } from './helpers.js';
import { ReleaseBook } from '../src/domain/releases.js';
import { DeploymentBook } from '../src/domain/deployments.js';

test('a target is parsed and its hostname is what the policy checks', () => {
  assert.deepEqual(assertTarget('https://hooks.example.com/relay'), { host: 'hooks.example.com', port: '', path: '/relay', url: 'https://hooks.example.com/relay' });
  assert.throws(() => assertTarget('https://127.0.0.1/relay'), (error) => error.status === 403 && /internal address/.test(error.message));
  assert.throws(() => assertTarget('https://user@127.0.0.1:9000/relay'), /127\.0\.0\.1 is an internal address/);
  assert.throws(() => assertTarget('http://hooks.example.com/relay'), /must use https/);
  assert.throws(() => assertTarget('not a url'), /is not a url/);
  assert.throws(() => assertTarget(''), /a target url is required/);
  assert.equal(isInternalHost('10.1.2.3'), true);
  assert.equal(isInternalHost('172.20.0.1'), true);
  assert.equal(isInternalHost('hooks.example.com'), false);
  assert.equal(hostAllowed('hooks.example.com', []), true);
  assert.equal(hostAllowed('hooks.example.com', ['example.com']), true);
  assert.equal(hostAllowed('evil.example.net', ['example.com']), false);
  assert.equal(hostOf('https://hooks.example.com/relay'), 'hooks.example.com');
  assert.deepEqual(describeDecision('https://127.0.0.1/relay').allowed, false);
  assert.deepEqual(policyReport().allowed, ALLOWED_HOSTS);
  assert.equal(parseTarget('https://hooks.example.com/relay').hostname, 'hooks.example.com');
});

test('a subscription stores the url its target check approved', () => {
  const book = new SubscriptionBook({ clock });
  const subscription = book.add({ url: 'https://hooks.example.com/relay', events: ['release.created', 'deployment.finished'], tenant: 'acme' });
  assert.equal(subscription.host, 'hooks.example.com');
  assert.deepEqual(subscription.events, ['release.created', 'deployment.finished']);
  assert.throws(() => book.add({ url: 'https://hooks.example.com/relay', events: ['release.created'], tenant: 'acme' }), /already subscribes/);
  assert.throws(() => book.add({ url: 'https://127.0.0.1/relay', events: ['release.created'] }), /internal address/);
  assert.throws(() => book.add({ url: 'https://hooks.example.com/x', events: ['nope'] }), /at least one of/);
  assert.equal(book.targets('acme', 'release.created').length, 1);
  assert.equal(book.targets('acme', 'incident.opened').length, 0);
  assert.deepEqual(subscriptionSummary(subscription).host, 'hooks.example.com');
  book.disable(subscription.id);
  assert.equal(book.targets('acme', 'release.created').length, 0);
  assert.equal(book.list({ enabledOnly: true }).length, 0);
  assert.equal(book.stats().subscriptions, 1);
  assert.equal(WEBHOOK_EVENTS.includes('deployment.started'), true);
  assert.throws(() => normalizeSubscription({ url: 'https://x.example.com', events: [] }), /at least one/);
});

test('a restored subscription keeps its stored url for the delivery path to check', () => {
  const book = new SubscriptionBook({ clock });
  const restored = book.restore({ url: 'https://hooks.partner.example/relay', events: ['release.created'], tenant: 'acme' });
  assert.equal(restored.url, 'https://hooks.partner.example/relay');
  assert.equal(restored.host, 'hooks.partner.example');
  assert.throws(() => book.restore({ url: '', events: ['release.created'] }), /needs a url/);
  assert.equal(book.restoreAll([{ url: 'https://hooks.partner.example/two', events: ['incident.opened'] }]).length, 1);
});

test('a delivery signature verifies against the body and the clock', () => {
  const body = JSON.stringify({ event: 'release.created' });
  const now = new Date('2026-02-10T12:00:00.000Z').getTime();
  const header = signDelivery(body, 'secret-value', now);
  assert.equal(parseSignature(header).v1, signatureOf(`${Math.floor(now / 1000)}.${body}`, 'secret-value'));
  assert.equal(verifySignature(body, header, 'secret-value', { now }).ok, true);
  assert.equal(verifySignature(body, header, 'other-secret', { now }).ok, false);
  assert.equal(verifySignature(body, 'v1=abc', 'secret-value', { now }).ok, false);
  assert.equal(signatureAge(header, { now }), 0);
  const secrets = new SecretBook({});
  const issued = secrets.issue('sub_1');
  assert.equal(secrets.secretFor('sub_1'), issued.secret);
  assert.notEqual(secrets.rotate('sub_1').secret, issued.secret);
  assert.equal(secrets.forget('sub_1'), true);
  assert.equal(secrets.stats().secrets, 0);
});

test('a retry policy backs off and stops after its attempts', () => {
  const policy = normalizePolicy({ attempts: 3, baseSeconds: 10, factor: 2 });
  assert.equal(delaySeconds(policy, 1), 10);
  assert.equal(delaySeconds(policy, 2), 20);
  assert.equal(delaySeconds(policy, 3), 40);
  assert.deepEqual(attemptsFor(policy, '2026-02-10T12:00:00.000Z').map((row) => row.delay_seconds), [10, 20, 40]);
  assert.equal(nextAttemptAt(policy, 1, '2026-02-10T12:00:00.000Z'), '2026-02-10T12:00:10.000Z');
  assert.equal(isExhausted(policy, 3), true);
  assert.equal(isExhausted(policy, 2), false);
  assert.equal(describePolicy(policy).schedule.length, 3);
  assert.equal(scheduleTable(policy, '2026-02-10T12:00:00.000Z')[0].minutes, 0.17);
  const tracker = new RetryTracker({ policy, clock });
  assert.deepEqual(tracker.record('dlv_1', 'timeout'), { attempt: 1, exhausted: false, next: '2026-02-10T12:00:10.000Z' });
  tracker.record('dlv_1', 'timeout');
  const last = tracker.record('dlv_1', 'timeout');
  assert.equal(last.exhausted, true);
  assert.equal(tracker.countFor('dlv_1'), 3);
  assert.equal(tracker.stats().deliveries, 1);
});

test('a delivery records its host, its attempts and its outcome', () => {
  const deliveries = new DeliveryLog({ clock });
  const subscription = { id: 'sub_1', tenant: 'acme', host: 'hooks.example.com', url: 'https://hooks.example.com/relay' };
  const delivery = deliveries.queue(subscription, 'release.created', { version: '1.4.0' });
  assert.equal(delivery.state, 'queued');
  assert.equal(deliveries.pending().length, 1);
  const posted = deliveries.attempt(delivery.id, { ok: true, status: 202 });
  assert.equal(posted.state, 'posted');
  assert.equal(posted.attempts, 1);
  assert.equal(attemptHistory(posted).length, 1);
  const second = deliveries.queue(subscription, 'deployment.finished', {});
  deliveries.attempt(second.id, { ok: false, error: 'timeout' });
  assert.equal(deliveries.list({ state: 'failed' }).length, 1);
  assert.equal(deliveries.byHost()['hooks.example.com'], 2);
  assert.deepEqual(successByHost(deliveries.list({ limit: 10 }))[0].host, 'hooks.example.com');
  assert.deepEqual(deliverySummary(deliveries.list({ limit: 10 })).success_rate, 50);
  assert.equal(retryCandidates(deliveries.list({ limit: 10 }), 4).length, 1);
  assert.equal(deliveryReport(deliveries.list({ limit: 10 })).byEvent['release.created'], 1);
  assert.equal(deliveries.purge('2026-02-11T00:00:00.000Z'), 2);
  assert.equal(deliveries.stats().deliveries, 0);
});

test('the sender prepares a post to an allowlisted host', async () => {
  const recorder = recordingTransport();
  const sender = new WebhookSender({ clock, transport: recorder.transport, allowed: [] });
  const plan = sender.prepare({ url: 'https://hooks.example.com/relay', payload: { event: 'release.created' }, secret: 'secret-value' });
  assert.equal(plan.host, 'hooks.example.com');
  assert.equal(plan.headers['x-dashboard-signature'] !== undefined, true);
  const result = await sender.send({ url: 'https://hooks.example.com/relay', payload: { event: 'release.created' }, secret: 'secret-value' });
  assert.equal(result.status, 202);
  assert.equal(recorder.posted.length, 1);
  assert.throws(() => sender.prepare({ url: 'https://127.0.0.1/relay' }), /internal address/);
  assert.throws(() => sender.prepare({ url: 'http://hooks.example.com/relay' }), /https|scheme/);
  const restricted = new WebhookSender({ clock, transport: recorder.transport, allowed: ['example.com'] });
  assert.throws(() => restricted.prepare({ url: 'https://hooks.example.net/relay' }), /not an allowlisted host/);
  assert.equal(sender.describe().timeoutMs, 5000);
});

test('a payload names the event and the record it carries', () => {
  const releases = new ReleaseBook({ clock });
  const release = releases.create({ ...releaseInput(), tenant: 'acme' });
  const deployments = new DeploymentBook({ clock });
  const deployment = deployments.start(release, { actor: 'usr_1' });
  assert.equal(releasePayload(release, { tenant: 'acme' }).event, 'release.created');
  assert.equal(deploymentPayload(deployment, { tenant: 'acme' }).data.service, 'checkout');
  assert.equal(buildPayload('incident.opened', { id: 'inc_1' }).event, 'incident.opened');
  assert.throws(() => buildPayload('nope'), /unknown webhook event/);
  assert.equal(describePayload(buildPayload('release.created', { version: '1.0.0' })).rows, 1);
  assert.equal(payloadEvents().length, WEBHOOK_EVENTS.length);
});
