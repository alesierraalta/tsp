import { json } from '../http/response.js';
import { readJson, pageParams } from '../http/request.js';
import { requireAdmin, requireRead, requireWebhooks } from '../auth/principal.js';
import { SubscriptionBook, subscriptionSummary } from '../webhooks/subscriptions.js';
import { DeliveryLog, deliverySummary } from '../webhooks/delivery.js';
import { WebhookSender } from '../webhooks/sender.js';
import { policyReport, describeDecision } from '../webhooks/targets.js';
import { payloadEvents } from '../webhooks/payloads.js';
import { describePolicy } from '../webhooks/retry.js';

export function webhookRoutes(router, services) {
  const { subscriptions, deliveries, sender, retry, allowedHosts } = services;

  router.get('/webhooks', async (ctx) => {
    requireRead(ctx);
    json(ctx.res, 200, {
      items: subscriptions.list({ tenant: ctx.query.tenant }).map(subscriptionSummary),
      stats: subscriptions.stats(),
      deliveries: deliverySummary(deliveries.list({ limit: 20 })),
    });
  });

  router.post('/webhooks', async (ctx) => {
    requireWebhooks(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    const subscription = subscriptions.add({ ...body, tenant: body.tenant ?? ctx.principal.tenant });
    json(ctx.res, 201, subscriptionSummary(subscription));
  });

  router.post('/webhooks/:id/disable', async (ctx) => {
    requireWebhooks(ctx);
    json(ctx.res, 200, subscriptionSummary(subscriptions.disable(ctx.params.id)));
  });

  router.post('/webhooks/:id/enable', async (ctx) => {
    requireWebhooks(ctx);
    json(ctx.res, 200, subscriptionSummary(subscriptions.enable(ctx.params.id)));
  });

  router.delete('/webhooks/:id', async (ctx) => {
    requireWebhooks(ctx);
    json(ctx.res, 200, { removed: subscriptions.remove(ctx.params.id) });
  });

  router.get('/webhooks/deliveries', async (ctx) => {
    requireRead(ctx);
    const page = pageParams(ctx.query, { limit: 20 });
    json(ctx.res, 200, {
      items: deliveries.list({ tenant: ctx.query.tenant, state: ctx.query.state, limit: page.limit }),
      stats: deliveries.stats(),
      by_event: deliveries.byEvent(),
    });
  });

  router.get('/webhooks/events', async (ctx) => {
    requireRead(ctx);
    json(ctx.res, 200, { items: payloadEvents() });
  });

  router.get('/webhooks/policy', async (ctx) => {
    requireRead(ctx);
    json(ctx.res, 200, { ...policyReport({ allowed: allowedHosts }), retry: describePolicy(retry.policy) });
  });

  router.post('/webhooks/check', async (ctx) => {
    requireRead(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    const decision = describeDecision(body.url, { allowed: allowedHosts });
    json(ctx.res, decision.allowed ? 200 : 403, decision);
  });

  router.post('/webhooks/preview', async (ctx) => {
    requireRead(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    const plan = sender.prepare({ url: body.url, payload: body.payload ?? { event: 'release.created' }, subscription: body.subscription });
    json(ctx.res, 200, { host: plan.host, href: plan.href, key: plan.key });
  });

  router.post('/webhooks/sweep', async (ctx) => {
    requireAdmin(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    json(ctx.res, 200, { purged: deliveries.purge(body.before) });
  });
}

export function createWebhookServices(options = {}) {
  const allowed = options.allowedHosts ?? [];
  return {
    subscriptions: new SubscriptionBook({ clock: options.clock, allowed }),
    deliveries: new DeliveryLog({ clock: options.clock }),
    sender: new WebhookSender({ clock: options.clock, allowed, transport: options.transport }),
  };
}
