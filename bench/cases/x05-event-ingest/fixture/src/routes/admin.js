import { json } from '../http/response.js';
import { queryInt, readJson, readBody } from '../http/request.js';
import { requireAdmin, requireRead, requireWrite } from '../auth/principal.js';
import { retentionTable } from '../rollups/retention.js';
import { planRank, isUpgrade } from '../quota/plans.js';
import { dedupeEvents, collisionRate } from '../ingest/dedupe.js';
import { auditActors, auditSummary } from '../store/audit.js';
import { partitionWindow, partitionReport } from '../store/partitions.js';
import { queuePressure } from '../jobs/queue.js';

export function adminRoutes(router, services) {
  const { keys, plans, reservations, retention, queue, store, meter, schemas } = services;
  const { audit, partitions, snapshots, retentionWorker } = services;

  router.get('/admin/stats', async (ctx) => {
    requireRead(ctx);
    json(ctx.res, 200, {
      store: store.stats(),
      keys: keys.stats(),
      plans: plans.stats(),
      reservations: reservations.stats(),
      meter: meter.stats(),
      queue: queue.stats(),
    });
  });

  router.get('/admin/keys', async (ctx) => {
    requireAdmin(ctx);
    json(ctx.res, 200, { items: keys.list(ctx.query.tenant) });
  });

  router.post('/admin/keys', async (ctx) => {
    requireAdmin(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    json(ctx.res, 201, keys.issue(body.tenant, { label: body.label, scopes: body.scopes, expires_at: body.expires_at }));
  });

  router.post('/admin/keys/:id/revoke', async (ctx) => {
    requireAdmin(ctx);
    json(ctx.res, 200, { revoked: keys.revoke(ctx.params.id) });
  });

  router.get('/admin/plans', async (ctx) => {
    requireRead(ctx);
    json(ctx.res, 200, { items: plans.list() });
  });

  router.put('/admin/tenants/:tenant/plan', async (ctx) => {
    requireAdmin(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    const before = plans.for(ctx.params.tenant).name;
    const plan = plans.assign(ctx.params.tenant, body.plan);
    json(ctx.res, 200, { tenant: ctx.params.tenant, plan: plan.name, upgrade: isUpgrade(before, plan.name), rank: planRank(plan.name) });
  });

  router.get('/admin/retention', async (ctx) => {
    requireRead(ctx);
    json(ctx.res, 200, { items: retentionTable() });
  });

  router.post('/admin/retention/sweep', async (ctx) => {
    requireAdmin(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    json(ctx.res, 200, retention.sweep(body.tenant));
  });

  router.get('/admin/queue', async (ctx) => {
    requireAdmin(ctx);
    json(ctx.res, 200, { items: queue.list(ctx.query.status), stats: queue.stats() });
  });

  router.post('/admin/queue', async (ctx) => {
    requireAdmin(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    json(ctx.res, 201, queue.enqueue(body.kind ?? 'rollup', body.payload ?? {}));
  });

  router.post('/admin/queue/drain', async (ctx) => {
    requireAdmin(ctx);
    json(ctx.res, 200, { items: await queue.drain(async (job) => ({ kind: job.kind, done: true }), { limit: queryInt(ctx.query, 'limit', 25) }) });
  });

  router.post('/admin/dedupe/report', async (ctx) => {
    requireWrite(ctx);
    const body = await readBody(ctx.req, { limit: services.maxBodyBytes });
    const lines = body.toString('utf8').split('\n').filter((line) => line.trim() !== '');
    const rows = lines.map((line, index) => {
      try {
        return JSON.parse(line);
      } catch (error) {
        throw new TypeError(`line ${index + 1} is not valid JSON: ${error.message}`);
      }
    });
    const { events, duplicates } = dedupeEvents(rows, (row) => row.id ?? row.event_id);
    json(ctx.res, 200, { kept: events.length, duplicates: duplicates.length, rate: collisionRate(rows, (row) => row.id ?? row.event_id) });
  });

  router.get('/admin/schemas', async (ctx) => {
    requireRead(ctx);
    json(ctx.res, 200, { items: schemas.list() });
  });

  // The audit log, the partition index and the snapshot book are what a retention review reads before it
  // decides to sweep: the plan endpoint reports the rows a sweep would remove without removing them.
  router.get('/admin/audit', async (ctx) => {
    requireRead(ctx);
    json(ctx.res, 200, {
      items: audit.list({ tenant: ctx.query.tenant, action: ctx.query.action, limit: queryInt(ctx.query, 'limit', 50) }),
      summary: auditSummary(audit),
      actors: auditActors(audit, ctx.query.tenant),
    });
  });

  router.post('/admin/audit/prune', async (ctx) => {
    requireAdmin(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    if (body.before === undefined) throw new TypeError('a prune needs the instant to prune before');
    json(ctx.res, 200, { removed: audit.prune(body.before) });
  });

  router.get('/admin/partitions', async (ctx) => {
    requireRead(ctx);
    const tenant = ctx.query.tenant ?? ctx.principal?.tenant ?? 'default';
    json(ctx.res, 200, { items: partitions.daySizes(tenant), window: partitionWindow(partitions, tenant), report: partitionReport(partitions) });
  });

  router.post('/admin/partitions/compact', async (ctx) => {
    requireAdmin(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    json(ctx.res, 200, { compacted: partitions.compactAll(body.tenant ?? ctx.principal.tenant) });
  });

  router.get('/admin/snapshots', async (ctx) => {
    requireRead(ctx);
    json(ctx.res, 200, { items: snapshots.list(), stats: snapshots.stats() });
  });

  router.post('/admin/retention/plan', async (ctx) => {
    requireRead(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes }).catch(() => ({}));
    json(ctx.res, 200, retentionWorker.plan(body.tenant ?? ctx.principal.tenant, { steps: body.steps }));
  });

  router.post('/admin/retention/run', async (ctx) => {
    requireAdmin(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes }).catch(() => ({}));
    json(ctx.res, 200, retentionWorker.sweep(body.tenant ?? ctx.principal.tenant, { steps: body.steps, dry_run: body.dry_run }));
  });

  router.get('/admin/queue/pressure', async (ctx) => {
    requireRead(ctx);
    json(ctx.res, 200, queuePressure(queue));
  });
}
