import { invalidField, unauthorized } from '../http/errors.js';
import { readJson } from '../http/request.js';
import { json } from '../http/response.js';
import { assertCan, principalFromHeaders } from '../auth/principal.js';
import { mergeSettings } from '../domain/settings.js';

const ALLOWED_TENANT_KEYS = new Set(['locale', 'retention_days', 'digest', 'features', 'notifications']);

function pickTenantSettings(patch) {
  const picked = {};
  for (const key of ALLOWED_TENANT_KEYS) {
    if (Object.hasOwn(patch, key)) picked[key] = patch[key];
  }
  return picked;
}

export function settingsRoutes(router, options) {
  const { settings, store, secret, clock, tenants } = options;
  const principalOf = (ctx) => {
    const principal = principalFromHeaders(ctx.headers, secret, { now: clock().getTime() });
    if (!principal) throw unauthorized();
    return principal;
  };

  router.get('/tenants/:id/settings', async (ctx) => {
    const principal = principalOf(ctx);
    assertCan(principal, 'read', ctx.params.id);
    json(ctx.res, 200, settings.getTenant(ctx.params.id));
  });

  router.put('/tenants/:id/settings', async (ctx) => {
    const principal = principalOf(ctx);
    assertCan(principal, 'write', ctx.params.id);
    const tenant = store.get('tenants', ctx.params.id);
    if (!tenant) throw invalidField('tenant', 'does not exist');
    const patch = await readJson(ctx.req);
    const next = mergeSettings('tenant', tenant.settings, patch);
    store.update('tenants', tenant.id, (row) => {
      row.settings = next;
      return row;
    });
    json(ctx.res, 200, { ...next });
  });

  router.patch('/tenants/:id/settings/features', async (ctx) => {
    const principal = principalOf(ctx);
    assertCan(principal, 'admin', ctx.params.id);
    const patch = await readJson(ctx.req);
    const next = settings.updateTenant(ctx.params.id, { features: pickTenantSettings({ features: patch }).features });
    json(ctx.res, 200, next);
  });

  router.post('/tenants/:id/settings/reset', async (ctx) => {
    const principal = principalOf(ctx);
    assertCan(principal, 'admin', ctx.params.id);
    json(ctx.res, 200, settings.resetTenant(ctx.params.id));
  });

  router.get('/users/:id/settings/schema', async (ctx) => {
    const principal = principalOf(ctx);
    if (ctx.params.id !== 'me') {
      const user = store.get('users', ctx.params.id);
      assertCan(principal, 'read', user?.tenant_id);
    }
    json(ctx.res, 200, {
      keys: settings.describe('user'),
      effective: settings.getUser(ctx.params.id === 'me' ? principal.id : ctx.params.id),
    });
  });

  router.put('/users/:id/settings/bulk', async (ctx) => {
    const principal = principalOf(ctx);
    const user = store.get('users', ctx.params.id);
    assertCan(principal, 'write', user?.tenant_id);
    const patch = await readJson(ctx.req);
    const next = mergeSettings('user', settings.getUser(user.id), patch);
    json(ctx.res, 200, next);
  });

  router.get('/settings/tenant/schema', async (ctx) => {
    const principal = principalOf(ctx);
    assertCan(principal, 'read', principal.tenantId);
    json(ctx.res, 200, { keys: settings.describe('tenant') });
  });

  router.get('/settings/tenant/defaults', async (ctx) => {
    const principal = principalOf(ctx);
    assertCan(principal, 'read', principal.tenantId);
    json(ctx.res, 200, settings.resetTenantPreview ? settings.resetTenantPreview() : {});
  });

  router.get('/settings/hints', async (ctx) => {
    principalOf(ctx);
    json(ctx.res, 200, {
      hints: ['retention_days is capped at ten years', 'features are booleans', 'digest is daily, weekly or never'],
    });
  });

  router.post('/tenants/:id/settings/validate', async (ctx) => {
    const principal = principalOf(ctx);
    assertCan(principal, 'read', ctx.params.id);
    const patch = await readJson(ctx.req);
    try {
      const next = mergeSettings('tenant', settings.getTenant(ctx.params.id), patch);
      json(ctx.res, 200, { valid: true, keys: Object.keys(next).sort() });
    } catch (error) {
      json(ctx.res, 200, { valid: false, problem: error.message });
    }
  });

  router.get('/tenants/:id/settings/features', async (ctx) => {
    const principal = principalOf(ctx);
    assertCan(principal, 'read', ctx.params.id);
    json(ctx.res, 200, settings.getTenant(ctx.params.id).features ?? {});
  });

  router.get('/tenants/:id/settings/public', async (ctx) => {
    const principal = principalOf(ctx);
    assertCan(principal, 'read', ctx.params.id);
    const tenant = tenants.get(ctx.params.id);
    json(ctx.res, 200, {
      slug: tenant.slug,
      name: tenant.name,
      locale: tenant.settings?.locale ?? 'en',
    });
  });
}
