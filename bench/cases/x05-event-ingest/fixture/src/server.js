import { createServer } from 'node:http';
import { MemoryStore } from './store/memory.js';
import { SeriesStore } from './store/series.js';
import { KeyRegistry } from './auth/keys.js';
import { principalFromHeaders } from './auth/principal.js';
import { PlanBook } from './quota/plans.js';
import { ReservationBook } from './quota/reserve.js';
import { SchemaBook, defaultTypes } from './ingest/schemas.js';
import { Intake } from './ingest/intake.js';
import { NdjsonIntake } from './ingest/ndjson.js';
import { DedupeWindow } from './ingest/dedupe.js';
import { Sessionizer } from './sessions/sessionizer.js';
import { Meter } from './billing/meter.js';
import { InvoiceBook } from './billing/invoice.js';
import { DeliveryLog } from './webhooks/delivery.js';
import { RuleBook } from './webhooks/rules.js';
import { ReportForwarder } from './reports/forward.js';
import { RetentionPolicy } from './rollups/retention.js';
import { JobQueue } from './jobs/queue.js';
import { RetentionWorker } from './jobs/retention.js';
import { PartnerBook } from './reports/partners.js';
import { AuditLog } from './store/audit.js';
import { PartitionedStore } from './store/partitions.js';
import { SnapshotBook } from './store/snapshots.js';
import { Router } from './http/router.js';
import { json, sendError } from './http/response.js';
import { parseHeaders } from './http/headers.js';
import { clientOf, queryOf } from './http/request.js';
import { createLogger } from './util/log.js';
import { ingestRoutes } from './routes/ingest.js';
import { reportRoutes } from './routes/reports.js';
import { webhookRoutes } from './routes/webhooks.js';
import { adminRoutes } from './routes/admin.js';
import { partnerRoutes } from './routes/partners.js';

export function createApp(options = {}) {
  const clock = options.clock ?? (() => new Date());
  const logger = options.logger ?? createLogger({ level: options.logLevel ?? 'silent' });
  const store = options.store ?? new MemoryStore({ clock });
  const series = new SeriesStore({ clock });
  const keys = new KeyRegistry({ clock });
  const plans = new PlanBook();
  const reservations = new ReservationBook({ plans, clock });
  const schemas = new SchemaBook(options.types ?? defaultTypes());
  const dedupe = new DedupeWindow({ limit: options.dedupeLimit ?? 10000 });
  const intake = new Intake({ store, plans, schemas, dedupe, series, clock });
  const ndjsonIntake = new NdjsonIntake({ intake, plans });
  const sessionizer = new Sessionizer({ gapSeconds: options.sessionGapSeconds });
  const meter = new Meter({ clock });
  const invoices = new InvoiceBook({ clock });
  const deliveries = new DeliveryLog({ clock });
  const rules = new RuleBook({});
  const forwarder = new ReportForwarder({ clock, allowed: options.allowedHosts });
  const retention = new RetentionPolicy({ clock, store, plans });
  const queue = new JobQueue({ clock });
  const partitions = new PartitionedStore({ clock });
  const snapshots = new SnapshotBook({ clock });
  const audit = new AuditLog({ clock });
  const partners = new PartnerBook({ clock });
  const retentionWorker = new RetentionWorker({ clock, plans, store, partitions, series, snapshots, audit, queue });
  const router = new Router();

  router.use(async (ctx, next) => {
    ctx.startedAt = Date.now();
    try {
      ctx.principal = principalFromHeaders(ctx.headers, keys);
    } catch {
      ctx.principal = undefined;
    }
    await next();
    logger.debug('request', { method: ctx.method, path: ctx.path, status: ctx.res.statusCode, ms: Date.now() - ctx.startedAt });
  });

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
    series,
    keys,
    plans,
    reservations,
    schemas,
    intake,
    ndjsonIntake,
    dedupe,
    sessionizer,
    meter,
    invoices,
    deliveries,
    rules,
    forwarder,
    retention,
    retentionWorker,
    partners,
    partitions,
    snapshots,
    audit,
    queue,
    logger,
    clock,
    maxBodyBytes: options.maxBodyBytes ?? 1024 * 1024,
  };

  ingestRoutes(router, services);
  reportRoutes(router, services);
  webhookRoutes(router, services);
  adminRoutes(router, services);
  partnerRoutes(router, services);

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
    };
    try {
      await router.handle(ctx);
      if (!res.writableEnded) json(res, 404, { error: 'not_found', message: `no route for ${ctx.method} ${ctx.path}` });
    } catch (error) {
      if (!res.writableEnded) sendError(res, error, { logger });
    }
  };

  return { router, store, series, keys, plans, reservations, schemas, intake, sessionizer, meter, invoices, deliveries, rules, forwarder, retention, retentionWorker, partners, partitions, snapshots, audit, queue, services, logger, handler };
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
