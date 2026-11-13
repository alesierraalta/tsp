import { json, noContent } from '../http/response.js';
import { readJson, rangeOf } from '../http/request.js';
import { requireAdmin, requireRead, requireWrite } from '../auth/principal.js';
import { contentTypeView } from '../domain/content-types.js';
import { entryListView } from '../http/serialize.js';

export function contentTypeRoutes(router, services) {
  const { schemas, entries } = services;

  router.get('/content-types', async (ctx) => {
    requireRead(ctx);
    json(ctx.res, 200, { items: schemas.list() });
  });

  router.get('/content-types/:apiId', async (ctx) => {
    requireRead(ctx);
    json(ctx.res, 200, contentTypeView(schemas.get(ctx.params.apiId)));
  });

  router.post('/content-types', async (ctx) => {
    requireAdmin(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    const schema = schemas.define(body);
    json(ctx.res, 201, contentTypeView(schema));
  });

  router.patch('/content-types/:apiId', async (ctx) => {
    requireAdmin(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    const schema = schemas.update(ctx.params.apiId, body);
    json(ctx.res, 200, contentTypeView(schema));
  });

  router.delete('/content-types/:apiId', async (ctx) => {
    requireAdmin(ctx);
    const schema = schemas.get(ctx.params.apiId);
    const count = entries.list(schema).length;
    if (count > 0) {
      json(ctx.res, 409, { error: 'content_type_not_empty', message: `${count} entries still use ${schema.api_id}` });
      return;
    }
    schemas.remove(schema.api_id);
    noContent(ctx.res);
  });

  router.get('/content-types/:apiId/entries', async (ctx) => {
    requireRead(ctx);
    const schema = schemas.get(ctx.params.apiId);
    const records = entries.list(schema, { locale: ctx.query.locale, status: ctx.query.status });
    const range = rangeOf(ctx.query, records.length);
    json(ctx.res, 200, {
      items: entryListView(records.slice(range.offset, range.end)),
      total: records.length,
      limit: range.limit,
      offset: range.offset,
      has_more: range.end < records.length,
    });
  });

  router.post('/content-types/:apiId/validate', async (ctx) => {
    requireWrite(ctx);
    const schema = schemas.get(ctx.params.apiId);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    const { validateEntry } = await import('../domain/validation.js');
    const result = validateEntry(schema, body.values ?? body, { partial: body.partial === true });
    json(ctx.res, result.ok ? 200 : 422, { ok: result.ok, errors: result.errors });
  });
}
