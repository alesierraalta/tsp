import { readJson, requireFields } from '../http/request.js';
import { created, json, noContent } from '../http/response.js';
import { assertCan, assertSameTenant, isSelf, principalFromHeaders } from '../auth/principal.js';
import { badRequest } from '../http/errors.js';

export function userRoutes(router, options) {
  const { users, secret, clock } = options;
  const principalOf = (ctx) => principalFromHeaders(ctx.headers, secret, { now: clock().getTime() });

  router.get('/tenants/:id/users', async (ctx) => {
    const principal = principalOf(ctx);
    assertCan(principal, 'read', ctx.params.id);
    json(ctx.res, 200, users.list(ctx.params.id, ctx.query));
  });

  router.post('/tenants/:id/users', async (ctx) => {
    const principal = principalOf(ctx);
    assertCan(principal, 'admin', ctx.params.id);
    const body = await readJson(ctx.req);
    requireFields(body, ['email', 'name']);
    const user = users.create({ ...body, tenant_id: ctx.params.id }, principal.id || 'unknown');
    created(ctx.res, users.present(user), `/users/${user.id}`);
  });

  router.get('/users/:id', async (ctx) => {
    const principal = principalOf(ctx);
    const user = users.get(ctx.params.id);
    assertCan(principal, 'read', user.tenant_id);
    if (!isSelf(principal, user.id) && !principal.roles.includes('admin')) {
      assertCan(principal, 'admin', user.tenant_id);
    }
    json(ctx.res, 200, users.present(user));
  });

  router.put('/users/:id', async (ctx) => {
    const principal = principalOf(ctx);
    const user = users.get(ctx.params.id);
    assertCan(principal, 'write', user.tenant_id);
    if (!isSelf(principal, user.id)) assertCan(principal, 'admin', user.tenant_id);
    const body = await readJson(ctx.req);
    if (body.name === undefined) throw badRequest('only the name can be changed here');
    const updated = users.rename(user.id, body.name, principal.id || 'unknown');
    json(ctx.res, 200, users.present(updated));
  });

  router.put('/users/:id/roles', async (ctx) => {
    const principal = principalOf(ctx);
    const user = users.get(ctx.params.id);
    assertCan(principal, 'admin', user.tenant_id);
    const body = await readJson(ctx.req);
    requireFields(body, ['roles']);
    const updated = users.setRoles(user.id, body.roles, principal.id || 'unknown', principal);
    json(ctx.res, 200, users.present(updated));
  });

  router.get('/users/:id/settings', async (ctx) => {
    const principal = principalOf(ctx);
    const user = users.get(ctx.params.id);
    assertSameTenant(principal, user.tenant_id);
    if (!isSelf(principal, user.id)) assertCan(principal, 'read', user.tenant_id);
    json(ctx.res, 200, users.settingsOf(user.id));
  });

  router.put('/users/:id/settings', async (ctx) => {
    const principal = principalOf(ctx);
    const user = users.get(ctx.params.id);
    assertSameTenant(principal, user.tenant_id);
    if (!isSelf(principal, user.id)) assertCan(principal, 'write', user.tenant_id);
    const body = await readJson(ctx.req);
    json(ctx.res, 200, users.updateSettings(user.id, body));
  });

  router.delete('/users/:id', async (ctx) => {
    const principal = principalOf(ctx);
    const user = users.get(ctx.params.id);
    assertCan(principal, 'admin', user.tenant_id);
    users.deactivate(user.id, principal.id || 'unknown');
    noContent(ctx.res);
  });

  router.get('/users/:id/sessions', async (ctx) => {
    const principal = principalOf(ctx);
    const user = users.get(ctx.params.id);
    assertSameTenant(principal, user.tenant_id);
    json(ctx.res, 200, { rows: [] });
  });
}
