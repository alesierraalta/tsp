import test from 'node:test';
import assert from 'node:assert/strict';
import { writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { SECRET, call, seedTenant, seedUser, tokenFor, withApp } from './helpers.js';

const PLATFORM = tokenFor({ sub: 'usr_ops', tenant: '', roles: ['platform'] });

test('health and stats answer without a token', async () => {
  await withApp(async ({ url }) => {
    const health = await call(url, 'GET', '/health');
    assert.equal(health.status, 200);
    assert.equal(health.json.status, 'ok');
    const stats = await call(url, 'GET', '/stats');
    assert.equal(stats.json.tenants, 0);
  });
});

test('whoami names the caller and refuses an anonymous one', async () => {
  await withApp(async ({ url, app }) => {
    const tenant = seedTenant(app);
    const owner = tokenFor({ sub: 'usr_owner', tenant: tenant.id, roles: ['owner'] });
    const named = await call(url, 'GET', '/whoami', { token: owner });
    assert.equal(named.json.tenant, tenant.id);
    assert.deepEqual(named.json.roles, ['owner']);
    const anonymous = await call(url, 'GET', '/whoami');
    assert.equal(anonymous.status, 401);
  });
});

test('a platform token creates tenants and sees all of them', async () => {
  await withApp(async ({ url }) => {
    const first = await call(url, 'POST', '/tenants', {
      token: PLATFORM,
      body: { name: 'Acme', plan: 'team', settings: { retention_days: 60 } },
    });
    assert.equal(first.status, 201);
    assert.equal(first.json.slug, 'acme');
    assert.equal(first.headers.get('location'), `/tenants/${first.json.id}`);

    const second = await call(url, 'POST', '/tenants', {
      token: PLATFORM,
      body: { name: 'Globex', plan: 'free' },
    });
    assert.equal(second.status, 201);

    const listed = await call(url, 'GET', '/tenants', { token: PLATFORM });
    assert.equal(listed.status, 200);
    assert.equal(listed.json.total, 2);
    assert.equal(listed.json.rows.every((row) => row.slug.length > 0), true);
  });
});

test('a tenant token only reaches its own tenant', async () => {
  await withApp(async ({ url, app }) => {
    const mine = seedTenant(app, { name: 'Mine' });
    const other = seedTenant(app, { name: 'Other' });
    const owner = tokenFor({ sub: 'usr_owner', tenant: mine.id, roles: ['owner'] });

    const own = await call(url, 'GET', `/tenants/${mine.id}`, { token: owner });
    assert.equal(own.status, 200);
    assert.equal(own.json.slug, 'mine');

    const foreign = await call(url, 'GET', `/tenants/${other.id}`, { token: owner });
    assert.equal(foreign.status, 403);

    const ownList = await call(url, 'GET', '/tenants', { token: owner });
    assert.equal(ownList.json.total, 1);
    assert.equal(ownList.json.rows[0].id, mine.id);
  });
});

test('renaming a tenant only moves the declared fields', async () => {
  await withApp(async ({ url, app }) => {
    const tenant = seedTenant(app);
    const owner = tokenFor({ sub: 'usr_owner', tenant: tenant.id, roles: ['owner'] });
    const renamed = await call(url, 'PUT', `/tenants/${tenant.id}`, {
      token: owner,
      body: { name: 'Acme Two', plan: 'business', ignored: 'x' },
    });
    assert.equal(renamed.status, 200);
    assert.equal(renamed.json.name, 'Acme Two');
    assert.equal(renamed.json.plan, 'business');

    const broken = await call(url, 'PUT', `/tenants/${tenant.id}`, { token: owner, body: { plan: 'gold' } });
    assert.equal(broken.status, 400);
    assert.equal(broken.json.error, 'invalid_field');
  });
});

test('a tenant settings write validates against the schema', async () => {
  await withApp(async ({ url, app }) => {
    const tenant = seedTenant(app);
    const owner = tokenFor({ sub: 'usr_owner', tenant: tenant.id, roles: ['owner'] });
    const written = await call(url, 'PUT', `/tenants/${tenant.id}/settings`, {
      token: owner,
      body: { retention_days: 90, digest: 'daily' },
    });
    assert.equal(written.status, 200);
    assert.equal(written.json.retention_days, 90);
    assert.equal(written.json.digest, 'daily');

    const invalid = await call(url, 'PATCH', `/tenants/${tenant.id}/settings/features`, {
      token: owner,
      body: { audit_log: 'yes' },
    });
    assert.equal(invalid.status, 400);
    assert.equal(invalid.json.error, 'invalid_field');
  });
});

test('a settings body naming __proto__ is refused', async () => {
  await withApp(async ({ url, app }) => {
    const tenant = seedTenant(app);
    const user = seedUser(app, tenant.id);
    const owner = tokenFor({ sub: 'usr_owner', tenant: tenant.id, roles: ['owner'] });

    const refused = await call(url, 'PUT', `/users/${user.id}/settings`, {
      token: owner,
      raw: '{"__proto__":{"admin":true}}',
    });
    assert.equal(refused.status, 400);
    assert.equal(refused.json.error, 'invalid_field');
    assert.equal({}.admin, undefined);

    const nested = await call(url, 'PUT', `/users/${user.id}/settings/bulk`, {
      token: owner,
      raw: '{"theme":{"__proto__":{"admin":true}}}',
    });
    assert.equal(nested.status, 400);
    assert.equal({}.admin, undefined);
  });
});

test('the tenant feature patch merges into the stored features', async () => {
  await withApp(async ({ url, app }) => {
    const tenant = seedTenant(app);
    const owner = tokenFor({ sub: 'usr_owner', tenant: tenant.id, roles: ['owner'] });
    const patched = await call(url, 'PATCH', `/tenants/${tenant.id}/settings/features`, {
      token: owner,
      body: { api_tokens: true },
    });
    assert.equal(patched.status, 200);
    assert.equal(patched.json.features.api_tokens, true);
    assert.equal(patched.json.features.audit_log, true);

    const features = await call(url, 'GET', `/tenants/${tenant.id}/settings/features`, { token: owner });
    assert.equal(features.json.api_tokens, true);
  });
});

test('a member can read settings and cannot patch features', async () => {
  await withApp(async ({ url, app }) => {
    const tenant = seedTenant(app);
    const member = seedUser(app, tenant.id, { email: 'member@example.com' });
    const memberToken = tokenFor({ sub: member.id, tenant: tenant.id, roles: ['member'] });

    const read = await call(url, 'GET', `/tenants/${tenant.id}/settings`, { token: memberToken });
    assert.equal(read.status, 200);
    assert.equal(read.json.locale, 'en');

    const patch = await call(url, 'PATCH', `/tenants/${tenant.id}/settings/features`, {
      token: memberToken,
      body: { exports: false },
    });
    assert.equal(patch.status, 403);
  });
});

test('user lifecycle through the API', async () => {
  await withApp(async ({ url, app }) => {
    const tenant = seedTenant(app);
    const admin = seedUser(app, tenant.id, {
      email: 'admin@example.com',
      roles: ['admin'],
    });
    const adminToken = tokenFor({ sub: admin.id, tenant: tenant.id, roles: ['admin'] });

    const created = await call(url, 'POST', `/tenants/${tenant.id}/users`, {
      token: adminToken,
      body: { email: '  New@Example.com ', name: 'New Person', roles: ['member'] },
    });
    assert.equal(created.status, 201);
    assert.equal(created.json.email, 'new@example.com');

    const renamed = await call(url, 'PUT', `/users/${created.json.id}`, {
      token: adminToken,
      body: { name: 'New Name' },
    });
    assert.equal(renamed.json.name, 'New Name');

    const roleChange = await call(url, 'PUT', `/users/${created.json.id}/roles`, {
      token: adminToken,
      body: { roles: ['viewer'] },
    });
    assert.deepEqual(roleChange.json.roles, ['viewer']);

    const removed = await call(url, 'DELETE', `/users/${created.json.id}`, { token: adminToken });
    assert.equal(removed.status, 204);
    const gone = await call(url, 'GET', `/users/${created.json.id}`, { token: adminToken });
    assert.equal(gone.json.active, false);
  });
});

test('reports are written, listed and downloaded', async () => {
  await withApp(async ({ url, dir, app }) => {
    await writeFile(join(dir, 'index.json'), JSON.stringify({ reports: [{ name: 'tenants' }] }));
    const tenant = seedTenant(app);
    const owner = tokenFor({ sub: 'usr_owner', tenant: tenant.id, roles: ['owner'] });

    const listed = await call(url, 'GET', '/reports', { token: owner });
    assert.equal(listed.status, 200);
    assert.deepEqual(listed.json.listed, [{ name: 'tenants' }]);

    const written = await call(url, 'POST', '/reports/tenants', { token: owner, body: {} });
    assert.equal(written.status, 201);

    const fetched = await call(url, 'GET', '/reports/tenants', { token: owner });
    assert.equal(fetched.status, 200);
    assert.match(fetched.text, /# Tenant report/);
  });
});

test('documents are written, read and removed inside the root', async () => {
  await withApp(async ({ url, app }) => {
    const tenant = seedTenant(app);
    const owner = tokenFor({ sub: 'usr_owner', tenant: tenant.id, roles: ['owner'] });

    const written = await call(url, 'PUT', '/documents/notes.txt', {
      token: owner,
      body: { text: 'hello' },
    });
    assert.equal(written.status, 201);
    assert.equal(written.json.size, 5);

    const read = await call(url, 'GET', '/documents/notes.txt', { token: owner });
    assert.equal(read.status, 200);
    assert.equal(read.text, 'hello');

    const listed = await call(url, 'GET', '/documents', { token: owner });
    assert.equal(listed.json.rows.some((entry) => entry.name === 'notes.txt'), true);

    const refused = await call(url, 'GET', '/documents/%2E%2E%2Fpackage.json', { token: owner });
    assert.equal(refused.status, 400);

    const removed = await call(url, 'DELETE', '/documents/notes.txt', { token: owner });
    assert.equal(removed.status, 200);
  });
});

test('archive entries round trip and export as csv', async () => {
  await withApp(async ({ url, app }) => {
    const tenant = seedTenant(app);
    const admin = tokenFor({ sub: 'usr_admin', tenant: tenant.id, roles: ['admin'] });

    const written = await call(url, 'PUT', '/reports/archive/daily.json', {
      token: admin,
      body: { rows: [{ slug: 'acme', users: 2 }] },
    });
    assert.equal(written.status, 201);

    const read = await call(url, 'GET', '/reports/archive/daily.json', { token: admin });
    assert.equal(read.json.rows[0].slug, 'acme');

    const listed = await call(url, 'GET', '/reports/archive/list', { token: admin });
    assert.equal(listed.json.entries[0].name, 'daily.json');

    const exported = await call(url, 'GET', '/reports/export/daily.json', { token: admin });
    assert.equal(exported.status, 200);
    assert.match(exported.text, /slug,users/);
    assert.match(exported.text, /acme,2/);
  });
});

test('unknown routes and wrong methods answer with a json error', async () => {
  await withApp(async ({ url }) => {
    const missing = await call(url, 'GET', '/nope');
    assert.equal(missing.status, 404);
    assert.equal(missing.json.error, 'not_found');
    const wrong = await call(url, 'DELETE', '/health');
    assert.equal(wrong.status, 404);
  });
});

test('a request body that is not json is refused before the handler', async () => {
  await withApp(async ({ url }) => {
    const response = await call(url, 'POST', '/tenants', {
      token: PLATFORM,
      headers: { 'content-type': 'text/plain' },
      raw: 'name=Acme',
    });
    assert.equal(response.status, 415);
    assert.equal(response.json.error, 'unsupported_media');
  });
});

test('the app reports the secret it signs with', async () => {
  await withApp(async ({ app }) => {
    assert.equal(app.secret, SECRET);
  });
});
