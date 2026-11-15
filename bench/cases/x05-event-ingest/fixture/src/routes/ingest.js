import { json, accepted, ndjson } from '../http/response.js';
import { queryInt, queryList, readBody, readJson, rangeOf } from '../http/request.js';
import { requireRead, requireWrite, tenantOf } from '../auth/principal.js';
import { isNdjson } from '../http/headers.js';
import { limitReport } from '../ingest/limits.js';
import { paginate } from '../util/collections.js';

export function ingestRoutes(router, services) {
  const { intake, ndjsonIntake, plans, schemas, reservations, series, store } = services;

  router.get('/v1/limits', async (ctx) => {
    const tenant = tenantOf(ctx);
    json(ctx.res, 200, { ...limitReport(plans.for(tenant)), reservation: reservations.stats() });
  });

  router.get('/v1/event-types', async (ctx) => {
    requireRead(ctx);
    json(ctx.res, 200, { items: schemas.list() });
  });

  router.post('/v1/event-types', async (ctx) => {
    requireWrite(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    json(ctx.res, 201, schemas.define(body));
  });

  router.post('/v1/batch', async (ctx) => {
    const tenant = requireWrite(ctx).tenant;
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    const result = intake.accept(tenant, Array.isArray(body.events) ? body.events : []);
    accepted(ctx.res, result);
  });

  router.post('/v1/events', async (ctx) => {
    const tenant = requireWrite(ctx).tenant;
    if (isNdjson(ctx.headers)) {
      const body = await readBody(ctx.req, { limit: services.maxBodyBytes });
      json(ctx.res, 202, ndjsonIntake.submit(tenant, body.toString('utf8')));
      return;
    }
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    json(ctx.res, 202, intake.accept(tenant, [body]));
  });

  router.post('/v1/events/append', async (ctx) => {
    const tenant = requireWrite(ctx).tenant;
    const body = await readBody(ctx.req, { limit: services.maxBodyBytes });
    const result = ndjsonIntake.append(tenant, body.toString('utf8'), { offset: queryInt(ctx.query, 'offset', 0) });
    json(ctx.res, 202, result);
  });

  router.get('/v1/events', async (ctx) => {
    const tenant = requireRead(ctx).tenant;
    const names = queryList(ctx.query, 'name');
    const events = intake
      .eventsOf(tenant, { name: names.length === 1 ? names[0] : undefined })
      .filter((event) => names.length <= 1 || names.includes(event.name));
    const range = rangeOf(ctx.query, events.length);
    json(ctx.res, 200, {
      items: events.slice(range.offset, range.end),
      total: events.length,
      limit: range.limit,
      offset: range.offset,
    });
  });

  router.get('/v1/events/export', async (ctx) => {
    const tenant = requireRead(ctx).tenant;
    const events = intake.eventsOf(tenant);
    ndjson(ctx.res, 200, events);
  });

  router.get('/v1/counts', async (ctx) => {
    const tenant = requireRead(ctx).tenant;
    json(ctx.res, 200, intake.counts(tenant));
  });

  router.get('/v1/series', async (ctx) => {
    const tenant = requireRead(ctx).tenant;
    const metric = ctx.query.metric ?? 'page_view';
    const points = series.points(tenant, metric, ctx.query.dimension);
    const page = paginate(points, { limit: queryInt(ctx.query, 'limit', 100), offset: queryInt(ctx.query, 'offset', 0) });
    json(ctx.res, 200, { items: page.items, total: page.total, series: series.stats() });
  });

  router.post('/v1/reservations', async (ctx) => {
    const tenant = requireWrite(ctx).tenant;
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    json(ctx.res, 201, reservations.reserve(tenant, body));
  });

  router.post('/v1/reservations/plan', async (ctx) => {
    const tenant = requireWrite(ctx).tenant;
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    json(ctx.res, 201, reservations.planCapacity(tenant, body));
  });

  router.get('/v1/reservations', async (ctx) => {
    const tenant = requireRead(ctx).tenant;
    json(ctx.res, 200, { items: reservations.list(tenant) });
  });

  router.delete('/v1/reservations/:id', async (ctx) => {
    requireWrite(ctx);
    json(ctx.res, 200, { released: reservations.release(ctx.params.id) });
  });

  router.get('/v1/store', async (ctx) => {
    requireRead(ctx);
    json(ctx.res, 200, store.stats());
  });
}
