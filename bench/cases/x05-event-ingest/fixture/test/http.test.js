import test from 'node:test';
import assert from 'node:assert/strict';
import { withApp, call, apiKey, pageView } from './helpers.js';

function seed(app) {
  const first = app.intake.accept('acme', [
    pageView({ user: 'usr_1', path: '/pricing', body: { id: 'evt_a', timestamp: '2026-02-08T09:00:00.000Z', samples: [1, 2] } }),
  ]);
  const second = app.intake.accept('acme', [
    pageView({ user: 'usr_2', path: '/docs', body: { id: 'evt_b', timestamp: '2026-02-09T10:00:00.000Z' } }),
  ]);
  return [first, second];
}

test('health answers anyone and whoami needs the key', async () => {
  const started = await withApp();
  try {
    const health = await call(started.url, '/health');
    assert.equal(health.status, 200);
    assert.equal(health.body.status, 'ok');
    assert.equal((await call(started.url, '/whoami')).status, 401);
    const who = await call(started.url, '/whoami', { headers: apiKey(started.issued.reader.secret) });
    assert.equal(who.status, 200);
    assert.equal(who.body.tenant, 'acme');
    const malformed = await call(started.url, '/whoami', { headers: { 'x-api-key': 'nope' } });
    assert.equal(malformed.status, 401);
    assert.equal((await call(started.url, '/nope')).status, 404);
  } finally {
    await started.stop();
  }
});

test('a batch is accepted and counted back', async () => {
  const started = await withApp();
  try {
    const accepted = await call(started.url, '/v1/batch', {
      method: 'POST',
      headers: apiKey(started.issued.writer.secret),
      body: JSON.stringify({ events: [pageView({ body: { id: 'evt_http_1' } })] }),
    });
    assert.equal(accepted.status, 202);
    assert.equal(accepted.body.accepted, 1);
    const counts = await call(started.url, '/v1/counts', { headers: apiKey(started.issued.reader.secret) });
    assert.equal(counts.body.total, 1);
    assert.equal(counts.body.byName.page_view, 1);
    const listed = await call(started.url, '/v1/events?name=page_view', { headers: apiKey(started.issued.reader.secret) });
    assert.equal(listed.body.items.length, 1);
    const readable = await call(started.url, '/v1/events', { headers: apiKey(started.issued.writer.secret) });
    assert.equal(readable.status, 200);
    const forbidden = await call(started.url, '/v1/batch', {
      method: 'POST',
      headers: apiKey(started.issued.reader.secret),
      body: JSON.stringify({ events: [pageView()] }),
    });
    assert.equal(forbidden.status, 403);
  } finally {
    await started.stop();
  }
});

test('the limits route reports the plan and what is reserved', async () => {
  const started = await withApp();
  try {
    const limits = await call(started.url, '/v1/limits', { headers: apiKey(started.issued.reader.secret) });
    assert.equal(limits.status, 200);
    assert.equal(limits.body.plan, 'free');
    assert.equal(limits.body.rows, 500);
    assert.equal(limits.body.reservation.reservations, 0);
  } finally {
    await started.stop();
  }
});

test('a reservation inside the plan is written and released over http', async () => {
  const started = await withApp();
  try {
    started.app.plans.assign('acme', 'growth');
    const created = await call(started.url, '/v1/reservations', {
      method: 'POST',
      headers: apiKey(started.issued.writer.secret),
      body: JSON.stringify({ rows: 10, samples: 100 }),
    });
    assert.equal(created.status, 201);
    assert.equal(created.body.slots, 100);
    const listed = await call(started.url, '/v1/reservations', { headers: apiKey(started.issued.reader.secret) });
    assert.equal(listed.body.items.length, 1);
    const released = await call(started.url, `/v1/reservations/${created.body.id}`, {
      method: 'DELETE',
      headers: apiKey(started.issued.writer.secret),
    });
    assert.equal(released.body.released, true);
    const tooMany = await call(started.url, '/v1/reservations', {
      method: 'POST',
      headers: apiKey(started.issued.writer.secret),
      body: JSON.stringify({ rows: 10, samples: 900000 }),
    });
    assert.equal(tooMany.status, 400);
    assert.equal(tooMany.body.error, 'batch_too_large');
  } finally {
    await started.stop();
  }
});

test('reports answer the questions the dashboards ask', async () => {
  const started = await withApp();
  try {
    seed(started.app);
    const rollup = await call(started.url, '/v1/reports/rollup?size=day', { headers: apiKey(started.issued.reader.secret) });
    assert.equal(rollup.status, 200);
    assert.equal(rollup.body.points.length, 2);
    assert.equal(rollup.body.totals.events, 2);
    const exportReport = await call(started.url, '/v1/reports/export?size=day&format=csv', { headers: apiKey(started.issued.reader.secret) });
    assert.equal(exportReport.body.manifest.rows, 2);
    assert.equal(exportReport.body.files.length, 1);
    const usage = await call(started.url, '/v1/reports/usage', { headers: apiKey(started.issued.reader.secret) });
    assert.equal(usage.body.cost_cents, 0);
  } finally {
    await started.stop();
  }
});

test('an invoice is issued in whole cents', async () => {
  const started = await withApp();
  try {
    const issued = await call(started.url, '/v1/reports/invoices', {
      method: 'POST',
      headers: apiKey(started.issued.writer.secret),
      body: JSON.stringify({ lines: [{ metric: 'events', units: 2000 }] }),
    });
    assert.equal(issued.status, 201);
    assert.equal(issued.body.total_cents, 70);
    assert.equal(issued.body.total, '$0.70');
    const listed = await call(started.url, '/v1/reports/invoices', { headers: apiKey(started.issued.reader.secret) });
    assert.equal(listed.body.total_cents, 70);
  } finally {
    await started.stop();
  }
});

test('admin routes report the store, the queue and the audit log', async () => {
  const started = await withApp();
  try {
    const stats = await call(started.url, '/admin/stats', { headers: apiKey(started.issued.reader.secret) });
    assert.equal(stats.status, 200);
    assert.equal(stats.body.keys.keys, 4);
    const audit = await call(started.url, '/admin/audit', { headers: apiKey(started.issued.reader.secret) });
    assert.equal(audit.status, 200);
    assert.equal(audit.body.summary.rows, 0);
    const enqueued = await call(started.url, '/admin/queue', {
      method: 'POST',
      headers: apiKey(started.issued.admin.secret),
      body: JSON.stringify({ kind: 'rollup', payload: { events: 2 } }),
    });
    assert.equal(enqueued.status, 201);
    const queue = await call(started.url, '/admin/queue', { headers: apiKey(started.issued.admin.secret) });
    assert.equal(queue.body.stats.queued, 1);
    const drained = await call(started.url, '/admin/queue/drain?limit=5', { method: 'POST', headers: apiKey(started.issued.admin.secret), body: '{}' });
    assert.equal(drained.body.items.length, 1);
    const refused = await call(started.url, '/admin/keys', { headers: apiKey(started.issued.reader.secret) });
    assert.equal(refused.status, 403);
  } finally {
    await started.stop();
  }
});

test('a retention sweep is planned before it runs', async () => {
  const started = await withApp();
  try {
    seed(started.app);
    const planned = await call(started.url, '/admin/retention/plan', { method: 'POST', headers: apiKey(started.issued.reader.secret), body: '{}' });
    assert.equal(planned.status, 200);
    assert.equal(planned.body.dry_run, true);
    assert.equal(planned.body.steps.events.expired, 0);
    const applied = await call(started.url, '/admin/retention/run', { method: 'POST', headers: apiKey(started.issued.admin.secret), body: '{}' });
    assert.equal(applied.body.dry_run, false);
    assert.equal(applied.body.steps.events.expired, 0);
    const audit = await call(started.url, '/admin/audit?action=retention.swept', { headers: apiKey(started.issued.reader.secret) });
    assert.equal(audit.body.summary.rows, 1);
  } finally {
    await started.stop();
  }
});

test('a partner is defined and its export is written, then read back', async () => {
  const started = await withApp();
  try {
    seed(started.app);
    const defined = await call(started.url, '/v1/partners', {
      method: 'POST',
      headers: apiKey(started.issued.admin.secret),
      body: JSON.stringify({ name: 'Beta Metrics', kind: 'agency', fields: ['name', 'at'] }),
    });
    assert.equal(defined.status, 201);
    assert.equal(defined.body.partner, 'beta-metrics');
    const refused = await call(started.url, '/v1/partners', {
      method: 'POST',
      headers: apiKey(started.issued.admin.secret),
      body: JSON.stringify({ name: 'Sneaky', fields: ['context.user'] }),
    });
    assert.equal(refused.status, 400);
    assert.equal(refused.body.error, 'partner_field_blocked');
    const exported = await call(started.url, '/v1/partners/beta-metrics/exports', {
      method: 'POST',
      headers: apiKey(started.issued.writer.secret),
      body: JSON.stringify({ format: 'csv' }),
    });
    assert.equal(exported.status, 201);
    assert.equal(exported.body.manifest.rows, 2);
    const file = await call(started.url, `/v1/partners/beta-metrics/exports/${exported.body.name}`, { headers: apiKey(started.issued.reader.secret) });
    assert.equal(file.status, 200);
    assert.equal(String(file.body).startsWith('name,at'), true);
    const bundle = await call(started.url, '/v1/partners/beta-metrics/bundles', {
      method: 'POST',
      headers: apiKey(started.issued.writer.secret),
      body: JSON.stringify({ format: 'ndjson', size: 1 }),
    });
    assert.equal(bundle.body.files.length, 2);
  } finally {
    await started.stop();
  }
});

test('a webhook subscription is checked against the egress policy', async () => {
  const started = await withApp();
  try {
    const ok = await call(started.url, '/v1/hooks', {
      method: 'POST',
      headers: apiKey(started.issued.admin.secret),
      body: JSON.stringify({ url: 'https://hooks.example.com/relay', events: ['batch.accepted'] }),
    });
    assert.equal(ok.status, 201);
    assert.equal(ok.body.host, 'hooks.example.com');
    const blocked = await call(started.url, '/v1/hooks', {
      method: 'POST',
      headers: apiKey(started.issued.admin.secret),
      body: JSON.stringify({ url: 'https://127.0.0.1/relay', events: ['batch.accepted'] }),
    });
    assert.equal(blocked.status, 403);
    assert.match(blocked.body.message, /internal address/);
    const policy = await call(started.url, '/v1/targets/policy', { headers: apiKey(started.issued.reader.secret) });
    assert.equal(policy.body.allowed.includes('hooks.example.com'), true);
    const decision = await call(started.url, '/v1/targets/check', {
      method: 'POST',
      headers: apiKey(started.issued.reader.secret),
      body: JSON.stringify({ url: 'https://evil.example.net/relay' }),
    });
    assert.equal(decision.status, 403);
  } finally {
    await started.stop();
  }
});

test('a body past the configured limit is refused before it is read', async () => {
  const started = await withApp({ maxBodyBytes: 2048 });
  try {
    const big = await call(started.url, '/v1/batch', {
      method: 'POST',
      headers: apiKey(started.issued.writer.secret),
      body: JSON.stringify({ events: Array.from({ length: 40 }, (_, index) => pageView({ body: { id: `evt_${index}` } })) }),
    });
    assert.equal(big.status, 413);
    assert.equal(big.body.error, 'payload_too_large');
  } finally {
    await started.stop();
  }
});
