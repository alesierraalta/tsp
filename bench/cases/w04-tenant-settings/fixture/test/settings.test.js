import test from 'node:test';
import assert from 'node:assert/strict';
import { MemoryStore } from '../src/store/memory.js';
import {
  BLOCKED_KEYS,
  SettingsService,
  defaultsFor,
  mergeSettings,
} from '../src/domain/settings.js';
import { TenantService } from '../src/domain/tenants.js';
import { UserService, canonicalEmail } from '../src/domain/users.js';
import { listAudits } from '../src/domain/audit.js';

function services() {
  const store = new MemoryStore({ clock: () => new Date('2026-03-01T00:00:00Z') });
  const settings = new SettingsService({ store, clock: () => new Date('2026-03-01T00:00:00Z') });
  const tenants = new TenantService({ store, settings, clock: () => new Date('2026-03-01T00:00:00Z') });
  const users = new UserService({ store, settings, tenants, clock: () => new Date('2026-03-01T00:00:00Z') });
  return { store, settings, tenants, users };
}

test('defaults come from the schema', () => {
  const tenant = defaultsFor('tenant');
  assert.equal(tenant.locale, 'en');
  assert.equal(tenant.retention_days, 30);
  assert.equal(tenant.features.audit_log, true);
  assert.equal(tenant.features.api_tokens, false);
  assert.equal(defaultsFor('user').theme, 'system');
  assert.throws(() => defaultsFor('nope'), /unknown settings kind/);
});

test('merge copies declared keys and keeps the rest', () => {
  const base = defaultsFor('tenant');
  const next = mergeSettings('tenant', base, { retention_days: 90, digest: 'daily' });
  assert.equal(next.retention_days, 90);
  assert.equal(next.digest, 'daily');
  assert.equal(next.locale, 'en');
  assert.equal(base.retention_days, 30);
});

test('merge refuses prototype keys at the top level and nested', () => {
  for (const key of BLOCKED_KEYS) {
    assert.throws(() => mergeSettings('tenant', {}, { [key]: { admin: true } }), (error) => {
      assert.equal(error.status, 400);
      assert.equal(error.code, 'invalid_field');
      return true;
    });
    assert.throws(
      () => mergeSettings('tenant', {}, { features: { [key]: { admin: true } } }),
      /not a setting/,
    );
  }
  assert.equal({}.admin, undefined);
});

test('merge refuses undeclared keys and wrong types', () => {
  assert.throws(() => mergeSettings('tenant', {}, { billing: 3 }), /not a setting/);
  assert.throws(() => mergeSettings('tenant', {}, { retention_days: '90' }), /whole number/);
  assert.throws(() => mergeSettings('tenant', {}, { retention_days: 0 }), /at least 1/);
  assert.throws(() => mergeSettings('tenant', {}, { retention_days: 9999 }), /at most 3650/);
  assert.throws(() => mergeSettings('tenant', {}, { locale: 'english' }), /format/);
  assert.throws(() => mergeSettings('tenant', {}, { digest: 'hourly' }), /one of/);
  assert.throws(() => mergeSettings('tenant', {}, { notifications: { webhook_url: 'http://x.test' } }), /format/);
  assert.throws(() => mergeSettings('tenant', {}, { features: { audit_log: 'yes' } }), /true or false/);
});

test('nested merges keep sibling keys', () => {
  const base = mergeSettings('tenant', {}, { features: { api_tokens: true } });
  const next = mergeSettings('tenant', base, { features: { exports: false } });
  assert.equal(next.features.api_tokens, true);
  assert.equal(next.features.exports, false);
  assert.equal(next.features.audit_log, true);
});

test('the service updates tenant and user settings', () => {
  const { settings, tenants, users } = services();
  const tenant = tenants.create({ name: 'Acme', plan: 'team' });
  const updated = settings.updateTenant(tenant.id, { retention_days: 90 });
  assert.equal(updated.retention_days, 90);
  assert.equal(tenants.get(tenant.id).settings.retention_days, 90);

  const user = users.create({ tenant_id: tenant.id, email: 'a@example.com', name: 'A' });
  const userSettings = settings.updateUser(user.id, { theme: 'dark' });
  assert.equal(userSettings.theme, 'dark');
  assert.equal(users.get(user.id).settings.theme, 'dark');
  assert.equal(settings.getUser(user.id).density, 'comfortable');
});

test('reset restores the defaults', () => {
  const { settings, tenants } = services();
  const tenant = tenants.create({ name: 'Acme' });
  settings.updateTenant(tenant.id, { digest: 'never' });
  assert.equal(settings.resetTenant(tenant.id).digest, 'weekly');
  assert.throws(() => settings.getTenant('tnt_nope'), /not found/);
  assert.deepEqual(settings.describe('tenant'), [
    'digest',
    'features',
    'locale',
    'notifications',
    'retention_days',
  ]);
});

test('tenant create validates the plan and the slug', () => {
  const { tenants } = services();
  assert.throws(() => tenants.create({ name: 'A', plan: 'team' }), /between 2 and 120/);
  assert.throws(() => tenants.create({ name: 'Acme', plan: 'gold' }), /must be one of/);
  const tenant = tenants.create({ name: 'Café Works', plan: 'business' });
  assert.equal(tenant.slug, 'cafe-works');
  assert.throws(() => tenants.create({ name: 'Cafe Works' }), /already used/);
});

test('tenant update only moves the declared fields', () => {
  const { tenants } = services();
  const tenant = tenants.create({ name: 'Acme', plan: 'team' });
  const renamed = tenants.update(tenant.id, { name: 'Acme Two', plan: 'free', slug: 'acme-two' });
  assert.equal(renamed.name, 'Acme Two');
  assert.equal(renamed.plan, 'free');
  assert.equal(renamed.slug, 'acme-two');
  assert.throws(() => tenants.update(tenant.id, { name: 'x' }), /between 2 and 120/);
  assert.throws(() => tenants.update(tenant.id, { plan: 'gold' }), /must be one of/);
});

test('tenant list filters, sorts and paginates', () => {
  const { tenants } = services();
  tenants.create({ name: 'Acme', plan: 'team' });
  tenants.create({ name: 'Globex', plan: 'free' });
  tenants.create({ name: 'Initech', plan: 'team' });
  const all = tenants.list({});
  assert.equal(all.total, 3);
  assert.equal(all.rows.every((row) => row.users === 0), true);
  assert.equal(tenants.list({ plan: 'team' }).total, 2);
  assert.equal(tenants.list({ q: 'glob' }).rows[0].slug, 'globex');
  assert.equal(tenants.list({ limit: 1 }).rows.length, 1);
  assert.equal(tenants.list({ sort: 'slug', dir: 'asc' }).rows[0].slug, 'acme');
});

test('emails are canonicalized before uniqueness', () => {
  const { users, tenants } = services();
  const tenant = tenants.create({ name: 'Acme' });
  const user = users.create({ tenant_id: tenant.id, email: '  Sam@Example.COM ', name: 'Sam' });
  assert.equal(user.email, 'sam@example.com');
  assert.equal(canonicalEmail('SAM@example.com'), 'sam@example.com');
  assert.throws(
    () => users.create({ tenant_id: tenant.id, email: 'sam@example.com', name: 'Other' }),
    /already registered/,
  );
  assert.equal(users.byEmail('SAM@EXAMPLE.COM').id, user.id);
});

test('user create validates tenant, email and roles', () => {
  const { users, tenants } = services();
  const tenant = tenants.create({ name: 'Acme' });
  assert.throws(() => users.create({ tenant_id: 'tnt_nope', email: 'a@example.com', name: 'A' }), /not found/);
  assert.throws(() => users.create({ tenant_id: tenant.id, email: 'nope', name: 'A' }), /email/);
  assert.throws(() => users.create({ tenant_id: tenant.id, email: 'a@example.com', name: '' }), /between 1 and 120/);
  assert.throws(
    () => users.create({ tenant_id: tenant.id, email: 'b@example.com', name: 'B', roles: ['wizard'] }),
    /unknown role/,
  );
});

test('the last owner cannot lose the owner role', () => {
  const { users, tenants } = services();
  const tenant = tenants.create({ name: 'Acme' });
  const owner = users.create({
    tenant_id: tenant.id,
    email: 'owner@example.com',
    name: 'Owner',
    roles: ['owner'],
  });
  assert.throws(() => users.setRoles(owner.id, ['admin']), /last owner/);
  assert.deepEqual(users.get(owner.id).roles, ['owner']);
});

test('an actor cannot change their own roles', () => {
  const { users, tenants } = services();
  const tenant = tenants.create({ name: 'Acme' });
  const admin = users.create({
    tenant_id: tenant.id,
    email: 'admin@example.com',
    name: 'Admin',
    roles: ['admin'],
  });
  assert.throws(
    () => users.setRoles(admin.id, ['member'], 'someone', { id: admin.id, roles: ['admin'] }),
    /your own roles/,
  );
});

test('audit rows are written for the tenant lifecycle', () => {
  const { store, tenants } = services();
  const tenant = tenants.create({ name: 'Acme' });
  tenants.update(tenant.id, { plan: 'free' });
  const audits = listAudits(store, tenant.id);
  assert.equal(audits.length, 2);
  assert.equal(audits[0].action, 'tenant.update');
  assert.equal(audits[1].action, 'tenant.create');
  assert.equal(store.count('audits'), 2);
});

test('removing a tenant removes its users', () => {
  const { store, tenants, users } = services();
  const tenant = tenants.create({ name: 'Acme' });
  users.create({ tenant_id: tenant.id, email: 'a@example.com', name: 'A' });
  users.create({ tenant_id: tenant.id, email: 'b@example.com', name: 'B' });
  assert.equal(tenants.userCount(tenant.id), 2);
  const removed = tenants.remove(tenant.id);
  assert.deepEqual(removed, { id: tenant.id, removed: true, users: 2 });
  assert.equal(store.count('users'), 0);
  assert.equal(store.count('tenants'), 0);
});

test('deactivating a user hides them from the member list', () => {
  const { users, tenants } = services();
  const tenant = tenants.create({ name: 'Acme' });
  const user = users.create({ tenant_id: tenant.id, email: 'a@example.com', name: 'A' });
  assert.equal(users.members(tenant.id).length, 1);
  users.deactivate(user.id);
  assert.equal(users.members(tenant.id).length, 0);
  assert.equal(users.get(user.id).active, false);
});
