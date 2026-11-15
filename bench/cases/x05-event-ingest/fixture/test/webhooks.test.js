import test from 'node:test';
import assert from 'node:assert/strict';
import { assertTarget, parseTarget, isInternalHost, hostAllowed, policyReport, ALLOWED_HOSTS } from '../src/webhooks/targets.js';
import { sign, verify, parseSignature, signatureOf, signatureAge } from '../src/webhooks/signature.js';
import { RuleBook, normalizeRule, evaluateRules, ruleReport, mergeRules, thresholdsOf } from '../src/webhooks/rules.js';
import { DeliveryLog, DELIVERY_EVENTS, deliverySummary } from '../src/webhooks/delivery.js';
import { ReportForwarder, forwardStatuses, markForward, forwardReport, purgeForwards } from '../src/reports/forward.js';
import { clock } from './helpers.js';

test('a target is parsed, then its scheme, host and address are checked', () => {
  assert.deepEqual(assertTarget('https://hooks.example.com/relay'), {
    host: 'hooks.example.com',
    path: '/relay',
    url: 'https://hooks.example.com/relay',
  });
  assert.throws(() => assertTarget('https://127.0.0.1/hook'), (error) => error.status === 403 && /internal address/.test(error.message));
  assert.throws(() => assertTarget('https://evil.example.net/hook'), /not an allowlisted host/);
  assert.throws(() => assertTarget('http://hooks.example.com/hook'), /must use https/);
  assert.throws(() => assertTarget('not a url'), /is not a url/);
  assert.throws(() => assertTarget(''), /a target url is required/);
  assert.equal(isInternalHost('10.1.2.3'), true);
  assert.equal(isInternalHost('hooks.example.com'), false);
  assert.equal(hostAllowed('eu.hooks.example.com'), true);
  assert.equal(hostAllowed('hooks.example.com.evil.test'), false);
  assert.equal(parseTarget('https://ingest.example.com/x').hostname, 'ingest.example.com');
  assert.deepEqual(policyReport().allowed, ALLOWED_HOSTS);
});

test('a signature is checked against the payload and the clock', () => {
  const payload = { event: 'batch.accepted', rows: 2 };
  const now = new Date('2026-02-10T12:00:00.000Z').getTime();
  const header = sign(payload, 'secret-value', { now });
  assert.deepEqual(parseSignature(header).v1, signatureOf(payload, 'secret-value'));
  assert.equal(verify(payload, header, 'secret-value', { now }).ok, true);
  assert.equal(verify(payload, header, 'other-secret', { now }).ok, false);
  assert.equal(verify(payload, 'v1=abc', 'secret-value', { now }).ok, false);
  assert.equal(verify(payload, `t=${Math.floor(now / 1000) - 4000},v1=x`, 'secret-value', { now }).reason, 'the signature is outside the tolerance');
  assert.equal(signatureAge(header, { now }), 0);
});

test('a rule is only stored once its target passes the same check', () => {
  const book = new RuleBook({});
  const rule = book.add({ event: 'rollup.ready', url: 'https://hooks.example.com/rollup', threshold: 10 });
  assert.equal(rule.enabled, true);
  assert.throws(() => book.add({ event: 'rollup.ready', url: 'https://127.0.0.1/hook' }), /internal address/);
  assert.throws(() => book.add({ event: 'rollup.ready', url: '' }), /a rule needs a url/);
  assert.equal(book.matching('rollup.ready', 10).length, 1);
  assert.equal(book.matching('rollup.ready', 5).length, 0);
  book.disable(rule.id);
  assert.equal(book.matching('rollup.ready', 20).length, 0);
  assert.deepEqual(ruleReport(book.list()), { rules: 1, enabled: 0, byEvent: { 'rollup.ready': 1 } });
  assert.deepEqual(evaluateRules(book.list(), 'rollup.ready', 20).skipped, [{ id: rule.id, reason: 'disabled' }]);
  assert.deepEqual(normalizeRule({ url: 'https://hooks.example.com/x' }).event, 'batch.accepted');
  assert.deepEqual(thresholdsOf(mergeRules([rule], [normalizeRule({ id: 'rule_b', url: 'https://hooks.example.com/y', threshold: 2 })])), {
    [rule.id]: 10,
    rule_b: 2,
  });
});

test('a subscription keeps the host it was checked against and records its deliveries', async () => {
  const deliveries = new DeliveryLog({ clock });
  const subscription = deliveries.subscribe('acme', 'https://hooks.example.com/relay', ['batch.accepted', 'rollup.ready']);
  assert.equal(subscription.host, 'hooks.example.com');
  assert.deepEqual(subscription.events, ['batch.accepted', 'rollup.ready']);
  assert.throws(() => deliveries.subscribe('acme', 'https://127.0.0.1/x', ['batch.accepted']), /internal address/);
  assert.throws(() => deliveries.subscribe('acme', 'https://hooks.example.com/x', ['nope']), /at least one known event/);
  const rows = await deliveries.publish('acme', 'batch.accepted', { rows: 2 });
  assert.equal(rows.length, 1);
  assert.equal(rows[0].host, 'hooks.example.com');
  assert.equal(deliveries.list('acme').length, 1);
  assert.equal(deliveries.list('globex').length, 0);
  assert.equal(deliveries.retry(rows[0].id).attempts, 2);
  assert.equal(deliveries.attempts(rows[0].id).length, 2);
  assert.deepEqual(deliveries.byHost('acme'), { 'hooks.example.com': 1 });
  assert.deepEqual(deliverySummary(deliveries.list('acme')).byHost, { 'hooks.example.com': 1 });
  assert.equal(deliveries.describe('acme').length, 1);
  assert.equal(deliveries.stats().subscriptions, 1);
  assert.equal(deliveries.unsubscribe(subscription.id), true);
  assert.equal(DELIVERY_EVENTS.includes('invoice.issued'), true);
});

test('a forwarder refuses the same targets the subscription path refuses', () => {
  const forwarder = new ReportForwarder({ clock });
  const record = forwarder.forward('acme', 'https://hooks.example.com/reports', { report: 'rollup', events: 12 });
  assert.equal(record.host, 'hooks.example.com');
  assert.equal(record.status, 'queued');
  assert.equal(forwarder.inspect('https://127.0.0.1/reports').allowed, false);
  assert.throws(() => forwarder.assertAllowed('https://127.0.0.1/reports'), /internal address/);
  assert.equal(forwarder.history('acme').length, 1);
  assert.deepEqual(forwardStatuses(forwarder.history('acme')), { queued: 1 });
  assert.equal(markForward(forwarder, record.id, 'delivered').status, 'delivered');
  assert.deepEqual(forwardReport(forwarder, 'acme').statuses, { delivered: 1 });
  assert.equal(purgeForwards(forwarder, '2026-02-11T00:00:00.000Z'), 1);
  assert.equal(forwarder.stats().sent, 0);
});
