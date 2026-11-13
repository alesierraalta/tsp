import { json } from '../http/response.js';
import { queryInt, queryList, readJson } from '../http/request.js';
import { requireRead, requireWrite } from '../auth/principal.js';
import { rollup, densify, growthOf, peakOf, rollupTotals } from '../rollups/rollup.js';
import { describeFunnel, defineFunnel } from '../reports/funnels.js';
import { cohortRetention, cohortTable } from '../reports/cohorts.js';
import { bounceRate } from '../sessions/sessionizer.js';
import { windowSummary } from '../sessions/windows.js';
import { chunkedExport, exportSummary, manifestFor, reportRows } from '../reports/exports.js';

export function reportRoutes(router, services) {
  const { intake, sessionizer, meter, invoices, forwarder } = services;

  router.get('/v1/reports/rollup', async (ctx) => {
    const tenant = requireRead(ctx).tenant;
    const events = intake.eventsOf(tenant, { name: ctx.query.name });
    const points = rollup(events, { size: ctx.query.size ?? 'hour' });
    json(ctx.res, 200, { points, totals: rollupTotals(points), growth: growthOf(points), peak: peakOf(points) });
  });

  router.get('/v1/reports/dense', async (ctx) => {
    const tenant = requireRead(ctx).tenant;
    const events = intake.eventsOf(tenant);
    const points = rollup(events, { size: 'day' });
    json(ctx.res, 200, { points: densify(points, ctx.query.from, ctx.query.to, 'day') });
  });

  router.get('/v1/reports/sessions', async (ctx) => {
    const tenant = requireRead(ctx).tenant;
    const events = intake.eventsOf(tenant);
    const sessions = sessionizer.build(tenant, events);
    json(ctx.res, 200, {
      sessions: sessions.length,
      bounce: bounceRate(sessions),
      windows: windowSummary(events, { size: 'hour', count: 1 }),
    });
  });

  router.post('/v1/reports/funnel', async (ctx) => {
    const tenant = requireRead(ctx).tenant;
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    const funnel = defineFunnel(body.steps ?? ['page_view', 'purchase']);
    const events = intake.eventsOf(tenant);
    json(ctx.res, 200, { funnel: describeFunnel(events, funnel) });
  });

  router.get('/v1/reports/cohorts', async (ctx) => {
    const tenant = requireRead(ctx).tenant;
    const events = intake.eventsOf(tenant);
    json(ctx.res, 200, { cohorts: cohortTable(events), retention: cohortRetention(events, queryInt(ctx.query, 'days', 7)) });
  });

  router.get('/v1/reports/usage', async (ctx) => {
    const tenant = requireRead(ctx).tenant;
    const period = ctx.query.period ?? 'current';
    json(ctx.res, 200, {
      lines: meter.linesFor(tenant, period),
      cost_cents: meter.costCents(tenant, period),
      by_metric: meter.byMetric(tenant, period),
    });
  });

  router.get('/v1/reports/invoices', async (ctx) => {
    const tenant = requireRead(ctx).tenant;
    json(ctx.res, 200, { items: invoices.list(tenant), total_cents: invoices.totals(tenant) });
  });

  router.post('/v1/reports/invoices', async (ctx) => {
    const tenant = requireWrite(ctx).tenant;
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    json(ctx.res, 201, invoices.create(tenant, body.lines ?? [], body.period ?? 'current'));
  });

  router.post('/v1/reports/forward', async (ctx) => {
    const tenant = requireWrite(ctx).tenant;
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    const result = forwarder.forward(tenant, body.url, {
      report: body.report ?? 'rollup',
      days: queryInt(ctx.query, 'days', 7),
      events: intake.eventsOf(tenant).length,
    });
    json(ctx.res, 202, result);
  });

  router.get('/v1/reports/names', async (ctx) => {
    const tenant = requireRead(ctx).tenant;
    json(ctx.res, 200, { items: queryList(ctx.query, 'name').length === 0 ? Object.keys(intake.counts(tenant).byName) : [] });
  });

  // An export is written in chunks of whole rows and described by a manifest, so a caller can check the row
  // count and the byte count of what it received against what the report promised.
  router.get('/v1/reports/export', async (ctx) => {
    const tenant = requireRead(ctx).tenant;
    const events = intake.eventsOf(tenant);
    const points = rollup(events, { size: ctx.query.size ?? 'day' });
    const totals = rollupTotals(points).events;
    const rows = reportRows(tenant, points, totals);
    const format = ['csv', 'json', 'ndjson'].includes(ctx.query.format) ? ctx.query.format : 'csv';
    const files = chunkedExport(`${tenant}-rollup`, rows, format, queryInt(ctx.query, 'chunk', 500));
    json(ctx.res, 200, {
      manifest: manifestFor(`rollup-${ctx.query.size ?? 'day'}`, rows, format),
      files: files.map((file) => ({ name: file.name, rows: file.rows })),
      summary: exportSummary(files),
    });
  });
}
