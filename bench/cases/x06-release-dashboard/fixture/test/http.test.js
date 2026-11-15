import test from 'node:test';
import assert from 'node:assert/strict';
import { withApp, call, apiKey, releaseInput } from './helpers.js';

test('health answers anyone and whoami needs a key', async () => {
  const started = await withApp();
  try {
    const health = await call(started.url, '/health');
    assert.equal(health.status, 200);
    assert.equal(health.body.status, 'ok');
    assert.equal((await call(started.url, '/whoami')).status, 401);
    const who = await call(started.url, '/whoami', { headers: apiKey(started.issued.reader.secret) });
    assert.equal(who.status, 200);
    assert.equal(who.body.tenant, 'acme');
    assert.equal((await call(started.url, '/nope')).status, 404);
    const malformed = await call(started.url, '/whoami', { headers: { 'x-api-key': 'nope' } });
    assert.equal(malformed.status, 401);
  } finally {
    await started.stop();
  }
});

test('a release is created, listed and read back', async () => {
  const started = await withApp();
  try {
    const created = await call(started.url, '/releases', {
      method: 'POST',
      headers: apiKey(started.issued.releaser.secret),
      body: JSON.stringify(releaseInput()),
    });
    assert.equal(created.status, 201);
    assert.equal(created.body.version, '1.4.0');
    assert.equal(created.body.channel, 'stable');
    const listed = await call(started.url, '/releases', { headers: apiKey(started.issued.reader.secret) });
    assert.equal(listed.body.items.length, 1);
    assert.equal(listed.body.summary.services, 1);
    const read = await call(started.url, `/releases/${created.body.id}`, { headers: apiKey(started.issued.reader.secret) });
    assert.equal(read.body.release.service, 'checkout');
    const notes = await call(started.url, `/releases/${created.body.id}/notes`, { headers: apiKey(started.issued.reader.secret) });
    assert.equal(notes.body.notes.sections.length, 2);
    assert.equal(notes.body.markdown.includes('# checkout 1.4.0'), true);
  } finally {
    await started.stop();
  }
});

test('a duplicate release version and a bad version are refused', async () => {
  const started = await withApp();
  try {
    const body = JSON.stringify(releaseInput());
    await call(started.url, '/releases', { method: 'POST', headers: apiKey(started.issued.releaser.secret), body });
    const duplicate = await call(started.url, '/releases', { method: 'POST', headers: apiKey(started.issued.releaser.secret), body });
    assert.equal(duplicate.status, 409);
    assert.equal(duplicate.body.error, 'release_duplicate');
    const bad = await call(started.url, '/releases', {
      method: 'POST',
      headers: apiKey(started.issued.releaser.secret),
      body: JSON.stringify({ ...releaseInput(), version: 'nope' }),
    });
    assert.equal(bad.status, 400);
    assert.equal(bad.body.error, 'invalid_version');
    const forbidden = await call(started.url, '/releases', {
      method: 'POST',
      headers: apiKey(started.issued.reader.secret),
      body: JSON.stringify({ ...releaseInput(), version: '9.9.9' }),
    });
    assert.equal(forbidden.status, 403);
  } finally {
    await started.stop();
  }
});

test('a release moves through its statuses and a promotion changes its environment', async () => {
  const started = await withApp();
  try {
    const created = await call(started.url, '/releases', {
      method: 'POST',
      headers: apiKey(started.issued.releaser.secret),
      body: JSON.stringify(releaseInput()),
    });
    const staged = await call(started.url, `/releases/${created.body.id}/status`, {
      method: 'POST',
      headers: apiKey(started.issued.releaser.secret),
      body: JSON.stringify({ status: 'staged' }),
    });
    assert.equal(staged.body.status, 'staged');
    const illegal = await call(started.url, `/releases/${created.body.id}/status`, {
      method: 'POST',
      headers: apiKey(started.issued.releaser.secret),
      body: JSON.stringify({ status: 'live' }),
    });
    assert.equal(illegal.status, 409);
    assert.equal(illegal.body.error, 'invalid_transition');
    const promoted = await call(started.url, `/releases/${created.body.id}/promote`, {
      method: 'POST',
      headers: apiKey(started.issued.releaser.secret),
      body: '{}',
    });
    assert.equal(promoted.body.environment, 'staging');
  } finally {
    await started.stop();
  }
});

test('a deployment walks its stages over http', async () => {
  const started = await withApp();
  try {
    const created = await call(started.url, '/releases', {
      method: 'POST',
      headers: apiKey(started.issued.releaser.secret),
      body: JSON.stringify(releaseInput()),
    });
    const deployment = await call(started.url, '/deployments', {
      method: 'POST',
      headers: apiKey(started.issued.releaser.secret),
      body: JSON.stringify({ release: created.body.id }),
    });
    assert.equal(deployment.status, 201);
    assert.equal(deployment.body.stage, 'prepare');
    const advanced = await call(started.url, `/deployments/${deployment.body.id}/advance`, {
      method: 'POST',
      headers: apiKey(started.issued.releaser.secret),
      body: '{}',
    });
    assert.equal(advanced.body.stage, 'approve');
    const requested = await call(started.url, `/deployments/${deployment.body.id}/approve`, {
      method: 'POST',
      headers: apiKey(started.issued.admin.secret),
      body: JSON.stringify({ role: 'approver' }),
    });
    assert.equal(requested.status, 202);
    assert.equal(requested.body.state, 'requested');
    const granted = await call(started.url, `/deployments/${deployment.body.id}/approve`, {
      method: 'POST',
      headers: apiKey(started.issued.admin.secret),
      body: JSON.stringify({ role: 'approver' }),
    });
    assert.equal(granted.status, 200);
    const listed = await call(started.url, '/deployments', { headers: apiKey(started.issued.reader.secret) });
    assert.equal(listed.body.items.length, 1);
    assert.equal(listed.body.items[0].approvals, 1);
  } finally {
    await started.stop();
  }
});

test('a webhook subscription is checked and a delivery is prepared', async () => {
  const started = await withApp();
  try {
    const created = await call(started.url, '/webhooks', {
      method: 'POST',
      headers: apiKey(started.issued.hooks.secret),
      body: JSON.stringify({ url: 'https://hooks.example.com/relay', events: ['release.created'] }),
    });
    assert.equal(created.status, 201);
    assert.equal(created.body.host, 'hooks.example.com');
    const blocked = await call(started.url, '/webhooks', {
      method: 'POST',
      headers: apiKey(started.issued.hooks.secret),
      body: JSON.stringify({ url: 'https://127.0.0.1/relay', events: ['release.created'] }),
    });
    assert.equal(blocked.status, 403);
    assert.match(blocked.body.message, /internal address/);
    const listed = await call(started.url, '/webhooks', { headers: apiKey(started.issued.reader.secret) });
    assert.equal(listed.body.items.length, 1);
    const policy = await call(started.url, '/webhooks/policy', { headers: apiKey(started.issued.reader.secret) });
    assert.equal(policy.body.scheme, 'https');
    const decision = await call(started.url, '/webhooks/check', {
      method: 'POST',
      headers: apiKey(started.issued.reader.secret),
      body: JSON.stringify({ url: 'https://evil.example.net/relay' }),
    });
    assert.equal(decision.status, 200);
    const preview = await call(started.url, '/webhooks/preview', {
      method: 'POST',
      headers: apiKey(started.issued.reader.secret),
      body: JSON.stringify({ url: 'https://hooks.example.com/relay', payload: { event: 'release.created' } }),
    });
    assert.equal(preview.status, 200);
    assert.equal(preview.body.host, 'hooks.example.com');
  } finally {
    await started.stop();
  }
});

test('the dashboard renders html when the request accepts it', async () => {
  const started = await withApp();
  try {
    await call(started.url, '/releases', {
      method: 'POST',
      headers: apiKey(started.issued.releaser.secret),
      body: JSON.stringify(releaseInput()),
    });
    const json = await call(started.url, '/', { headers: apiKey(started.issued.reader.secret) });
    assert.equal(json.status, 200);
    assert.equal(json.body.summary.releases, 1);
    const html = await call(started.url, '/', { headers: { ...apiKey(started.issued.reader.secret), accept: 'text/html' } });
    assert.equal(html.body.html.startsWith('<!doctype html>'), true);
    assert.equal(html.body.html.includes('data-environment="development"'), true);
    const metrics = await call(started.url, '/dashboards/metrics', { headers: apiKey(started.issued.reader.secret) });
    assert.equal(metrics.body.table.length, 4);
    const summary = await call(started.url, '/dashboards/summary', { headers: apiKey(started.issued.reader.secret) });
    assert.equal(summary.body.releases.releases, 1);
    const readiness = await call(started.url, '/dashboards/readiness', { headers: apiKey(started.issued.reader.secret) });
    assert.equal(readiness.status, 200);
  } finally {
    await started.stop();
  }
});

test('an incident is opened and blocks a production promotion', async () => {
  const started = await withApp();
  try {
    const opened = await call(started.url, '/incidents', {
      method: 'POST',
      headers: apiKey(started.issued.releaser.secret),
      body: JSON.stringify({ title: 'Checkout is down', severity: 'sev1', service: 'checkout', environment: 'production' }),
    });
    assert.equal(opened.status, 201);
    assert.equal(opened.body.severity, 'sev1');
    const listed = await call(started.url, '/incidents', { headers: apiKey(started.issued.reader.secret) });
    assert.equal(listed.body.blocking, 1);
    const resolved = await call(started.url, `/incidents/${opened.body.id}/resolve`, {
      method: 'POST',
      headers: apiKey(started.issued.releaser.secret),
      body: JSON.stringify({ resolution: 'rolled back' }),
    });
    assert.equal(resolved.body.state, 'resolved');
    const twice = await call(started.url, `/incidents/${opened.body.id}/resolve`, {
      method: 'POST',
      headers: apiKey(started.issued.releaser.secret),
      body: JSON.stringify({}),
    });
    assert.equal(twice.status, 409);
  } finally {
    await started.stop();
  }
});

test('admin routes report the store, the queue and the policies', async () => {
  const started = await withApp();
  try {
    const stats = await call(started.url, '/admin/stats', { headers: apiKey(started.issued.reader.secret) });
    assert.equal(stats.status, 200);
    assert.equal(stats.body.keys.keys, 4);
    const policies = await call(started.url, '/admin/policies', { headers: apiKey(started.issued.reader.secret) });
    assert.equal(policies.body.items.length, 4);
    const decision = await call(started.url, '/admin/policies/check', {
      method: 'POST',
      headers: apiKey(started.issued.reader.secret),
      body: JSON.stringify({ kind: 'deploy', environment: 'production', role: 'releaser' }),
    });
    assert.equal(decision.body.allowed, false);
    const freeze = await call(started.url, '/admin/freezes', {
      method: 'POST',
      headers: apiKey(started.issued.admin.secret),
      body: JSON.stringify({ environment: 'production', from: '2026-02-10T00:00:00.000Z', to: '2026-02-12T00:00:00.000Z', reason: 'audit' }),
    });
    assert.equal(freeze.status, 201);
    const frozen = await call(started.url, '/admin/freezes', { headers: apiKey(started.issued.reader.secret) });
    assert.equal(frozen.body.items.length, 1);
    const keys = await call(started.url, '/admin/keys', { headers: apiKey(started.issued.admin.secret) });
    assert.equal(keys.body.items.length, 4);
    const refused = await call(started.url, '/admin/keys', { headers: apiKey(started.issued.reader.secret) });
    assert.equal(refused.status, 403);
  } finally {
    await started.stop();
  }
});

test('a change freeze refuses a production deployment', async () => {
  const started = await withApp();
  try {
    started.app.freezes.add({ environment: 'production', from: '2026-02-10T00:00:00.000Z', to: '2026-02-12T00:00:00.000Z', reason: 'audit' });
    const created = await call(started.url, '/releases', {
      method: 'POST',
      headers: apiKey(started.issued.releaser.secret),
      body: JSON.stringify(releaseInput({ environment: 'production' })),
    });
    const refused = await call(started.url, '/deployments', {
      method: 'POST',
      headers: apiKey(started.issued.admin.secret),
      body: JSON.stringify({ release: created.body.id, role: 'owner' }),
    });
    assert.equal(refused.status, 409);
    assert.equal(refused.body.error, 'change_freeze');
    const overridden = await call(started.url, '/deployments', {
      method: 'POST',
      headers: apiKey(started.issued.admin.secret),
      body: JSON.stringify({ release: created.body.id, role: 'owner', override: true }),
    });
    assert.equal(overridden.status, 201);
  } finally {
    await started.stop();
  }
});

test('a body past the configured limit is refused before it is read', async () => {
  const started = await withApp({ maxBodyBytes: 1024 });
  try {
    const big = await call(started.url, '/releases', {
      method: 'POST',
      headers: apiKey(started.issued.releaser.secret),
      body: JSON.stringify({ ...releaseInput(), summary: 'x'.repeat(4000) }),
    });
    assert.equal(big.status, 413);
    assert.equal(big.body.error, 'payload_too_large');
  } finally {
    await started.stop();
  }
});
