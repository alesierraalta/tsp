import { createServer } from 'node:http';
import { MemoryStore } from './store/memory.js';
import { EventLog } from './store/events.js';
import { KeyRegistry } from './auth/keys.js';
import { principalFromHeaders } from './auth/principal.js';
import { ServiceCatalog } from './domain/services.js';
import { ReleaseBook } from './domain/releases.js';
import { DeploymentBook } from './domain/deployments.js';
import { ApprovalBook } from './domain/approvals.js';
import { IncidentBook } from './domain/incidents.js';
import { SubscriptionBook } from './webhooks/subscriptions.js';
import { DeliveryLog } from './webhooks/delivery.js';
import { WebhookSender } from './webhooks/sender.js';
import { RetryTracker, normalizePolicy } from './webhooks/retry.js';
import { SecretBook } from './webhooks/signature.js';
import { JobQueue } from './jobs/queue.js';
import { Scheduler } from './jobs/scheduler.js';
import { Router } from './http/router.js';
import { json, sendError } from './http/response.js';
import { parseHeaders } from './http/headers.js';
import { clientOf, queryOf } from './http/request.js';
import { attachPrincipal, cors, securityHeaders, timing, wantsHtml } from './http/middleware.js';
import { createLogger } from './util/log.js';
import { releaseRoutes, channelRoutes } from './routes/releases.js';
import { deploymentRoutes } from './routes/deployments.js';
import { webhookRoutes } from './routes/webhooks.js';
import { dashboardRoutes, readinessRoutes, createEnvironmentRegistry } from './routes/dashboards.js';
import { incidentRoutes } from './routes/incidents.js';
import { adminRoutes, freezeRoutes } from './routes/admin.js';
import { FreezeCalendar } from './domain/freeze.js';
import { PolicyBook } from './domain/policies.js';
import { configFromEnv, describeConfig } from './config.js';

export function createApp(options = {}) {
  const clock = options.clock ?? (() => new Date());
  const config = options.config ?? configFromEnv();
  const logger = options.logger ?? createLogger({ level: options.logLevel ?? config.logLevel ?? 'silent' });
  const store = new MemoryStore({ clock });
  const events = new EventLog({ clock });
  const keys = new KeyRegistry({ clock });
  const catalog = new ServiceCatalog({ clock });
  const releases = new ReleaseBook({ clock, events });
  const deployments = new DeploymentBook({ clock, events });
  const approvals = new ApprovalBook({ clock });
  const incidents = new IncidentBook({ clock });
  const allowedHosts = options.allowedHosts ?? config.allowedHosts ?? [];
  const subscriptions = new SubscriptionBook({ clock, allowed: allowedHosts });
  const deliveries = new DeliveryLog({ clock });
  const secrets = new SecretBook({});
  const sender = new WebhookSender({ clock, allowed: allowedHosts, transport: options.transport });
  const retry = new RetryTracker({ clock, policy: normalizePolicy(options.retryPolicy) });
  const queue = new JobQueue({ clock });
  const scheduler = new Scheduler({ clock, queue });
  const environments = createEnvironmentRegistry({ labels: options.environmentLabels });
  const freezes = new FreezeCalendar({ clock });
  const policies = new PolicyBook({});

  const router = new Router();
  router.use(timing());
  router.use(securityHeaders());
  router.use(cors(options.corsOrigin ?? '*'));
  router.use(wantsHtml());
  router.use(
    attachPrincipal((ctx) => {
      try {
        return principalFromHeaders(ctx.headers, keys);
      } catch {
        return undefined;
      }
    }),
  );

  router.get('/health', async (ctx) => {
    json(ctx.res, 200, { status: 'ok', at: new Date(clock()).toISOString() });
  });

  router.get('/whoami', async (ctx) => {
    if (!ctx.principal) {
      json(ctx.res, 401, { error: 'unauthorized', message: 'an api key is required' });
      return;
    }
    json(ctx.res, 200, { key: ctx.principal.key, tenant: ctx.principal.tenant, scopes: ctx.principal.scopes });
  });

  const services = {
    store,
    events,
    keys,
    catalog,
    services: catalog,
    releases,
    deployments,
    approvals,
    incidents,
    subscriptions,
    deliveries,
    secrets,
    sender,
    retry,
    queue,
    scheduler,
    environments,
    freezes,
    policies,
    allowedHosts,
    maxBodyBytes: options.maxBodyBytes ?? config.maxBodyBytes ?? 512 * 1024,
    config,
    logger,
    clock,
    deliveryStats: () => deliveries.stats(),
  };

  releaseRoutes(router, services);
  channelRoutes(router, services);
  deploymentRoutes(router, services);
  webhookRoutes(router, services);
  dashboardRoutes(router, services);
  readinessRoutes(router, services);
  incidentRoutes(router, services);
  adminRoutes(router, services);
  freezeRoutes(router, services);

  const handler = async (req, res) => {
    const url = new URL(req.url ?? '/', `http://${req.headers.host ?? 'localhost'}`);
    const ctx = {
      req,
      res,
      method: String(req.method ?? 'GET').toUpperCase(),
      path: url.pathname,
      url,
      headers: parseHeaders(req.headers),
      query: queryOf(url),
      client: clientOf(req),
      wantsHtml: false,
    };
    try {
      await router.handle(ctx);
      if (!res.writableEnded) json(res, 404, { error: 'not_found', message: `no route for ${ctx.method} ${ctx.path}` });
    } catch (error) {
      if (!res.writableEnded) sendError(res, error, { logger });
    }
  };

  return {
    router,
    store,
    events,
    keys,
    catalog,
    releases,
    deployments,
    approvals,
    incidents,
    subscriptions,
    deliveries,
    sender,
    retry,
    queue,
    scheduler,
    environments,
    freezes,
    policies,
    services,
    config,
    logger,
    handler,
  };
}

export async function startApp(options = {}) {
  const app = createApp(options);
  const server = createServer(app.handler);
  await new Promise((resolve, reject) => {
    server.once('error', reject);
    server.listen(options.port ?? 0, '127.0.0.1', resolve);
  });
  const address = server.address();
  return {
    app,
    server,
    url: `http://127.0.0.1:${address.port}`,
    close: () => new Promise((resolve, reject) => server.close((error) => (error ? reject(error) : resolve()))),
  };
}

export function describeApp(app) {
  return {
    routes: app.router.count(),
    services: app.catalog.stats().services,
    releases: app.releases.stats().releases,
    config: describeConfig(app.config),
  };
}
