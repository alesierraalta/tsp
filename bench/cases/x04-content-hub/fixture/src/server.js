import { createServer } from 'node:http';
import { MemoryStore } from './store/memory.js';
import { RevisionLog } from './store/revisions.js';
import { SchemaRegistry, defaultSchemas } from './domain/schemas.js';
import { defineContentType } from './domain/content-types.js';
import { EntryService } from './domain/entries.js';
import { PublishWorkflow } from './domain/publishing.js';
import { LocaleSet } from './domain/locales.js';
import { MediaStore } from './media/store.js';
import { planVariants } from './media/variants.js';
import { imageSize } from './media/images.js';
import { TemplateLoader } from './templates/loader.js';
import { helperMap } from './templates/helpers.js';
import { EntryRenderer } from './pipeline/render.js';
import { HookBus } from './plugins/hooks.js';
import { PluginRegistry } from './plugins/registry.js';
import { PluginHost } from './plugins/host.js';
import { PluginAssetStore } from './plugins/assets.js';
import { SearchIndex } from './search/index.js';
import { Exporter } from './export/exporters.js';
import { Importer } from './export/importer.js';
import { JobQueue } from './pipeline/queue.js';
import { JobRunner } from './pipeline/jobs.js';
import { Scheduler } from './schedule/scheduler.js';
import { Router } from './http/router.js';
import { json, sendError } from './http/response.js';
import { parseHeaders } from './http/headers.js';
import { clientOf, queryOf } from './http/request.js';
import { principalFromHeaders } from './auth/principal.js';
import { createLogger } from './util/log.js';
import { contentTypeRoutes } from './routes/content-types.js';
import { entryRoutes } from './routes/entries.js';
import { mediaRoutes } from './routes/media.js';
import { pluginRoutes } from './routes/plugins.js';
import { searchRoutes } from './routes/search.js';
import { exportRoutes } from './routes/exports.js';
import { adminRoutes } from './routes/admin.js';

export function createApp(options = {}) {
  const clock = options.clock ?? (() => new Date());
  const logger = options.logger ?? createLogger({ level: options.logLevel ?? 'silent' });
  const secret = options.secret ?? 'development-secret';
  const maxBodyBytes = options.maxBodyBytes ?? 512 * 1024;
  const store = options.store ?? new MemoryStore({ clock });
  const revisions = new RevisionLog({ clock, limit: options.revisionLimit ?? 25 });
  const schemas = options.schemas ?? new SchemaRegistry(defaultSchemas().map((input) => defineContentType(input)));
  const entries = new EntryService({ store, clock, revisions });
  const workflow = new PublishWorkflow({ entries, clock });
  const locales = new LocaleSet(options.locales);
  const registry = new PluginRegistry();
  const bus = new HookBus({ registry, logger });
  const plugins = new PluginHost({ root: options.pluginRoot, registry, bus, logger });
  const pluginAssets = new PluginAssetStore({ root: options.pluginAssetRoot ?? options.pluginRoot, clock });
  const media = new MediaStore({ root: options.mediaRoot, clock, maxBytes: options.maxMediaBytes });
  const loader = new TemplateLoader({ root: options.templateRoot, helpers: helperMap() });
  const renderer = new EntryRenderer({ loader, bus });
  const search = new SearchIndex();
  const exporters = new Exporter({ entries, schemas, root: options.exportRoot, bus, clock });
  const importer = new Importer({ entries, schemas });
  const queue = new JobQueue({ clock });
  const jobs = new JobRunner({ queue, entries, renderer, exporters, search, plugins });
  const scheduler = new Scheduler({ clock });
  const router = new Router();

  bus.on('entry.afterCreate', async ({ entry }) => {
    search.add(entry);
    return undefined;
  });
  bus.on('entry.afterUpdate', async ({ entry }) => {
    search.remove(entry.id);
    search.add(entry);
    return undefined;
  });
  bus.on('entry.afterPublish', async ({ entry }) => {
    search.remove(entry.id);
    search.add(entry);
    return undefined;
  });

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
    json(ctx.res, 200, { id: ctx.principal.id, tenant: ctx.principal.tenant, roles: ctx.principal.roles });
  });

  router.get('/stats', async (ctx) => {
    json(ctx.res, 200, {
      entries: store.count('entries'),
      content_types: schemas.size(),
      plugins: registry.stats().total,
      revisions: revisions.size(),
    });
  });

  const services = {
    store,
    revisions,
    schemas,
    entries,
    workflow,
    locales,
    registry,
    bus,
    plugins,
    pluginAssets,
    media,
    loader,
    renderer,
    search,
    exporters,
    importer,
    queue,
    jobs,
    scheduler,
    logger,
    maxBodyBytes,
    secret,
    clock,
    planVariantsCache: (name, body, wanted) => planVariants(name, imageSize(name, body), wanted),
  };

  contentTypeRoutes(router, services);
  entryRoutes(router, services);
  mediaRoutes(router, services);
  pluginRoutes(router, services);
  searchRoutes(router, services);
  exportRoutes(router, services);
  adminRoutes(router, services);

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

  return {
    router,
    store,
    revisions,
    schemas,
    entries,
    workflow,
    locales,
    registry,
    bus,
    plugins,
    pluginAssets,
    media,
    loader,
    renderer,
    search,
    exporters,
    importer,
    queue,
    jobs,
    scheduler,
    services,
    secret,
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
