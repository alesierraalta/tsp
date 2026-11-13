import { json, noContent } from '../http/response.js';
import { readJson } from '../http/request.js';
import { requireAdmin, requireRead, tenantOf } from '../auth/principal.js';
import { deliverySummary } from '../webhooks/delivery.js';
import { policyReport } from '../webhooks/targets.js';

export function webhookRoutes(router, services) {
  const { deliveries, rules, forwarder } = services;

  router.get('/v1/hooks', async (ctx) => {
    const tenant = requireRead(ctx).tenant;
    json(ctx.res, 200, { items: deliveries.list(tenant), summary: deliverySummary(deliveries.list(tenant)), policy: policyReport() });
  });

  router.post('/v1/hooks', async (ctx) => {
    const tenant = tenantOf(ctx);
    requireAdmin(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    json(ctx.res, 201, deliveries.subscribe(tenant, body.url, body.events ?? []));
  });

  router.delete('/v1/hooks/:id', async (ctx) => {
    requireAdmin(ctx);
    noContent(ctx.res);
    deliveries.unsubscribe(ctx.params.id);
  });

  router.get('/v1/rules', async (ctx) => {
    requireRead(ctx);
    json(ctx.res, 200, { items: rules.list() });
  });

  router.post('/v1/rules', async (ctx) => {
    requireAdmin(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    json(ctx.res, 201, rules.add(body));
  });

  router.post('/v1/rules/:id/disable', async (ctx) => {
    requireAdmin(ctx);
    json(ctx.res, 200, rules.disable(ctx.params.id) ?? { error: 'not_found' });
  });

  router.get('/v1/targets/policy', async (ctx) => {
    requireRead(ctx);
    json(ctx.res, 200, policyReport());
  });

  router.post('/v1/targets/check', async (ctx) => {
    requireRead(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    const decision = forwarder.inspect(body.url);
    json(ctx.res, decision.allowed ? 200 : 403, decision);
  });
}
