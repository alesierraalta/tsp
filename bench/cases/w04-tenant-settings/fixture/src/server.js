import { createServer } from 'node:http';
import { MemoryStore } from './store/memory.js';
import { SnapshotStore } from './store/persist.js';
import { DocumentStore } from './files/documents.js';
import { ReportCatalog } from './reports/catalog.js';
import { ReportArchive } from './reports/archive.js';
import { SettingsService } from './domain/settings.js';
import { TenantService } from './domain/tenants.js';
import { UserService } from './domain/users.js';
import { Router } from './router.js';
import { createLogger } from './util/log.js';
import { json, sendError } from './http/response.js';
import { parseHeaders } from './http/headers.js';
import { clientOf, queryOf } from './http/request.js';
import { principalFromHeaders } from './auth/principal.js';
import { tenantRoutes } from './routes/tenants.js';
import { userRoutes } from './routes/users.js';
import { settingsRoutes } from './routes/settings.js';
import { reportRoutes } from './routes/reports.js';
import { newId } from './util/ids.js';

export function createApp(options = {}) {
  const clock = options.clock ?? (() => new Date());
  const logger = options.logger ?? createLogger({ level: options.logLevel ?? 'silent' });
  const secret = options.secret ?? 'development-secret';
  const store = options.store ?? new MemoryStore({ clock });
  const snapshots = options.snapshotDir ? new SnapshotStore({ dir: options.snapshotDir, clock }) : undefined;
  const settings = new SettingsService({ store, clock });
  const tenants = new TenantService({ store, settings, clock });
  const users = new UserService({ store, settings, clock, tenants });
  const router = new Router();

  router.use(async (ctx, next) => {
    ctx.startedAt = Date.now();
    try {
      ctx.principal = principalFromHeaders(ctx.headers, secret, { now: clock().getTime() });
    } catch {
      ctx.principal = undefined;
    }
    await next();
    logger.debug('request', {
      method: ctx.method,
      path: ctx.path,
      status: ctx.res.statusCode,
      ms: Date.now() - ctx.startedAt,
    });
  });

  router.get('/health', async (ctx) => {
    json(ctx.res, 200, { status: 'ok', time: new Date(clock()).toISOString() });
  });

  router.get('/whoami', async (ctx) => {
    if (!ctx.principal) {
      json(ctx.res, 401, { error: 'unauthorized', message: 'authentication required' });
      return;
    }
    json(ctx.res, 200, {
      id: ctx.principal.id,
      tenant: ctx.principal.tenantId,
      roles: ctx.principal.roles,
    });
  });

  router.get('/stats', async (ctx) => {
    json(ctx.res, 200, {
      tenants: store.count('tenants'),
      users: store.count('users'),
      audits: store.count('audits'),
      writes: store.writes,
    });
  });

  const context = { store, settings, tenants, users, secret, clock, logger };
  tenantRoutes(router, { ...context, users, audit: store });
  userRoutes(router, context);
  settingsRoutes(router, context);

  if (options.dataRoot) {
    const catalog = new ReportCatalog({ root: options.dataRoot });
    const archive = new ReportArchive({ root: options.archiveRoot ?? options.dataRoot });
    const documents = new DocumentStore({ root: options.documentRoot ?? options.dataRoot });
    reportRoutes(router, { ...context, catalog, archive, documents });
  }

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
      if (!res.writableEnded) {
        json(res, 404, { error: 'not_found', message: `no route for ${ctx.method} ${ctx.path}` });
      }
    } catch (error) {
      if (!res.writableEnded) sendError(res, error, { logger });
    }
  };

  return { router, store, settings, tenants, users, secret, logger, snapshots, handler };
}

export async function startApp(options = {}) {
  const app = createApp(options);
  const server = createServer(app.handler);
  await new Promise((resolve, reject) => {
    server.once('error', reject);
    server.listen(options.port ?? 0, '127.0.0.1', resolve);
  });
  const address = server.address();
  const url = `http://127.0.0.1:${address.port}`;
  return {
    app,
    server,
    url,
    close: () =>
      new Promise((resolve, reject) => {
        server.close((error) => (error ? reject(error) : resolve()));
      }),
  };
}

export function seedTenant(app, input = {}) {
  return app.tenants.create({
    name: input.name ?? 'Acme',
    plan: input.plan ?? 'team',
    settings: input.settings ?? {},
  });
}

export function seedUser(app, tenantId, input = {}) {
  return app.users.create({
    tenant_id: tenantId,
    email: input.email ?? `user-${newId('x').slice(2)}@example.com`,
    name: input.name ?? 'Sam Member',
    roles: input.roles ?? ['member'],
  });
}
