import { badRequest, notFound } from '../http/errors.js';
import { queryOf, readJson, requireFields } from '../http/request.js';
import { created, json, noContent } from '../http/response.js';
import { assertCan, assertPlatform, principalFromHeaders } from '../auth/principal.js';
import { listAudits, recordAudit } from '../domain/audit.js';

export function tenantRoutes(router, options) {
  const { tenants, users, secret, clock, audit } = options;

  const principalOf = (ctx) => principalFromHeaders(ctx.headers, secret, { now: clock().getTime() });

  router.get('/tenants', async (ctx) => {
    const principal = principalOf(ctx);
    if (principal.roles.includes('platform')) {
      return json(ctx.res, 200, tenants.list(ctx.query));
    }
    assertCan(principal, 'read', principal.tenantId);
    const tenant = tenants.get(principal.tenantId);
    return json(ctx.res, 200, { rows: [tenants.present(tenant)], total: 1, next: null });
  });

  router.get('/tenants/:id', async (ctx) => {
    const principal = principalOf(ctx);
    assertCan(principal, 'read', ctx.params.id);
    return json(ctx.res, 200, tenants.present(tenants.get(ctx.params.id)));
  });

  router.post('/tenants', async (ctx) => {
    const principal = principalOf(ctx);
    assertPlatform(principal);
    const body = await readJson(ctx.req);
    requireFields(body, ['name']);
    tenants.ensureCapacity();
    const tenant = tenants.create(body, principal.id || 'platform');
    if (audit) {
      recordAudit(audit, {
        tenantId: tenant.id,
        actor: principal.id || 'platform',
        action: 'tenant.created',
        target: tenant.id,
        at: clock(),
      });
    }
    created(ctx.res, tenants.present(tenant), `/tenants/${tenant.id}`);
  });

  router.put('/tenants/:id', async (ctx) => {
    const principal = principalOf(ctx);
    assertCan(principal, 'write', ctx.params.id);
    const body = await readJson(ctx.req);
    const tenant = tenants.update(ctx.params.id, body, principal.id || 'unknown');
    json(ctx.res, 200, tenants.present(tenant));
  });

  router.get('/tenants/:id/members', async (ctx) => {
    const principal = principalOf(ctx);
    assertCan(principal, 'read', ctx.params.id);
    json(ctx.res, 200, { rows: users.members(ctx.params.id) });
  });

  router.delete('/tenants/:id', async (ctx) => {
    const principal = principalOf(ctx);
    assertPlatform(principal);
    const body = await readJson(ctx.req).catch(() => ({}));
    const expected = body.slug ?? ctx.query.slug;
    const tenant = tenants.get(ctx.params.id);
    if (expected !== tenant.slug) {
      throw badRequest('the tenant slug must be repeated to remove it');
    }
    tenants.remove(tenant.id, principal.id || 'platform');
    noContent(ctx.res);
  });

  router.get('/tenants/:id/audit', async (ctx) => {
    const principal = principalOf(ctx);
    assertCan(principal, 'admin', ctx.params.id);
    const limit = Number(queryOf(ctx.url).limit ?? 25);
    json(ctx.res, 200, {
      rows: listAudits(audit, ctx.params.id, { limit: Number.isFinite(limit) ? limit : 25 }),
    });
  });

  router.get('/tenants/lookup/:slug', async (ctx) => {
    const principal = principalOf(ctx);
    assertPlatform(principal);
    const tenant = tenants.bySlug(ctx.params.slug);
    json(ctx.res, 200, tenants.present(tenant));
  });

  router.get('/tenants/:id/summary', async (ctx) => {
    const principal = principalOf(ctx);
    assertCan(principal, 'read', ctx.params.id);
    const tenant = tenants.get(ctx.params.id);
    if (!tenant) throw notFound(`tenant ${ctx.params.id}`);
    json(ctx.res, 200, {
      id: tenant.id,
      slug: tenant.slug,
      plan: tenant.plan,
      users: tenants.userCount(tenant.id),
      settings: Object.keys(tenant.settings ?? {}).sort(),
    });
  });
}
