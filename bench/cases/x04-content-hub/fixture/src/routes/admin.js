import { json } from '../http/response.js';
import { readJson, queryInt } from '../http/request.js';
import { requireAdmin, requireRead, requireWrite } from '../auth/principal.js';
import { jobSummary, JOB_KINDS } from '../pipeline/jobs.js';
import { listTools } from '../pipeline/tools.js';

export function adminRoutes(router, services) {
  const { store, queue, jobs, scheduler, registry, plugins, revisions } = services;

  router.get('/admin/stats', async (ctx) => {
    requireRead(ctx);
    json(ctx.res, 200, {
      store: store.stats(),
      jobs: queue.stats(),
      plugins: registry.stats(),
      revisions: revisions.size(),
      scheduler: scheduler.stats(),
    });
  });

  router.get('/admin/jobs', async (ctx) => {
    requireAdmin(ctx);
    const status = ctx.query.status;
    json(ctx.res, 200, { items: queue.list(status).map(jobSummary) });
  });

  router.post('/admin/jobs', async (ctx) => {
    requireAdmin(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    const job = jobs.submit(body.kind, body.payload ?? {}, { priority: queryInt(ctx.query, 'priority', 0) });
    json(ctx.res, 201, jobSummary(job));
  });

  router.post('/admin/jobs/drain', async (ctx) => {
    requireAdmin(ctx);
    const done = await jobs.drain({ limit: queryInt(ctx.query, 'limit', 25) });
    json(ctx.res, 200, { items: done.map(jobSummary) });
  });

  router.get('/admin/tasks', async (ctx) => {
    requireAdmin(ctx);
    json(ctx.res, 200, { items: scheduler.list() });
  });

  router.post('/admin/tasks', async (ctx) => {
    requireAdmin(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    json(ctx.res, 201, scheduler.add(body));
  });

  router.post('/admin/tasks/tick', async (ctx) => {
    requireAdmin(ctx);
    const results = await scheduler.tick(async (task) => jobs.submit(task.kind, task.payload ?? {}, {}));
    json(ctx.res, 200, { results });
  });

  router.get('/admin/tools', async (ctx) => {
    requireRead(ctx);
    json(ctx.res, 200, { items: listTools() });
  });

  router.get('/admin/plugins/errors', async (ctx) => {
    requireAdmin(ctx);
    json(ctx.res, 200, { items: plugins.errors });
  });

  router.post('/admin/render/:entryId', async (ctx) => {
    requireWrite(ctx);
    const entry = services.entries.get(ctx.params.entryId);
    const html = await services.renderer.renderEntry(entry, { template: ctx.query.template ?? 'entry.html' });
    json(ctx.res, 200, { entryId: entry.id, bytes: Buffer.byteLength(html), html });
  });

  router.get('/admin/job-kinds', async (ctx) => {
    requireRead(ctx);
    json(ctx.res, 200, { items: JOB_KINDS });
  });

  router.get('/admin/audit', async (ctx) => {
    requireAdmin(ctx);
    json(ctx.res, 200, {
      items: services.audit.list({ action: ctx.query.action, actor: ctx.query.actor, limit: queryInt(ctx.query, 'limit', 50) }),
      counts: services.audit.counts(),
    });
  });

  router.get('/admin/revisions', async (ctx) => {
    requireAdmin(ctx);
    json(ctx.res, 200, { items: revisions.recent(queryInt(ctx.query, 'limit', 20)), actions: revisions.actions() });
  });

  router.post('/admin/revisions/prune', async (ctx) => {
    requireAdmin(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    const removed = body.before ? revisions.pruneOlderThan(body.before) : 0;
    json(ctx.res, 200, { removed, remaining: revisions.size() });
  });

  router.get('/admin/collections', async (ctx) => {
    requireRead(ctx);
    const items = services.collections.list().map((collection) =>
      services.collections.summary(collection.id, store.all('entries')),
    );
    json(ctx.res, 200, { items });
  });

  router.post('/admin/collections', async (ctx) => {
    requireAdmin(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    json(ctx.res, 201, services.collections.define(body));
  });

  router.get('/admin/themes', async (ctx) => {
    requireRead(ctx);
    json(ctx.res, 200, { items: services.themes.list(), active: services.themes.active });
  });

  router.post('/admin/themes/:name/activate', async (ctx) => {
    requireAdmin(ctx);
    json(ctx.res, 200, services.themes.activate(ctx.params.name));
  });

  router.get('/admin/deliveries', async (ctx) => {
    requireAdmin(ctx);
    json(ctx.res, 200, { items: services.deliveries.list(queryInt(ctx.query, 'limit', 25)), stats: services.deliveries.stats() });
  });
}
