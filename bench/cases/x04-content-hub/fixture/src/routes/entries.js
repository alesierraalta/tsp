import { json, noContent } from '../http/response.js';
import { queryInt, readJson } from '../http/request.js';
import { requireRead, requireWrite } from '../auth/principal.js';
import { entryListView, entryView } from '../http/serialize.js';
import { paginate } from '../util/collections.js';

export function entryRoutes(router, services) {
  const { schemas, entries, workflow, revisions, bus } = services;

  router.get('/entries', async (ctx) => {
    requireRead(ctx);
    const schema = ctx.query.type ? schemas.get(ctx.query.type) : undefined;
    const records = schema
      ? entries.list(schema, { locale: ctx.query.locale, status: ctx.query.status })
      : services.store.all('entries');
    const page = paginate(records, { limit: queryInt(ctx.query, 'limit', 20), offset: queryInt(ctx.query, 'offset', 0) });
    json(ctx.res, 200, {
      items: entryListView(page.items),
      total: page.total,
      limit: page.limit,
      offset: page.offset,
      has_more: page.has_more,
    });
  });

  router.get('/entries/:id', async (ctx) => {
    requireRead(ctx);
    json(ctx.res, 200, entryView(entries.get(ctx.params.id), { includePublished: true }));
  });

  router.post('/entries', async (ctx) => {
    const principal = requireWrite(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    const schema = schemas.get(body.type ?? ctx.query.type);
    const created = await bus.emit('entry.beforeCreate', { schema, input: body, principal });
    const entry = entries.create(schema, created.input ?? body, {
      locale: body.locale,
      status: body.status === 'published' ? 'draft' : body.status,
      actor: principal.id,
    });
    await bus.emit('entry.afterCreate', { entry, principal });
    json(ctx.res, 201, entryView(entry, { includePublished: true }));
  });

  router.put('/entries/:id', async (ctx) => {
    const principal = requireWrite(ctx);
    const current = entries.get(ctx.params.id);
    const schema = schemas.get(current.type);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    const entry = entries.update(current.id, schema, body, { actor: principal.id });
    await bus.emit('entry.afterUpdate', { entry, principal });
    json(ctx.res, 200, entryView(entry, { includePublished: true }));
  });

  router.delete('/entries/:id', async (ctx) => {
    const principal = requireWrite(ctx);
    json(ctx.res, 200, entries.remove(ctx.params.id, { actor: principal.id }));
  });

  router.post('/entries/:id/publish', async (ctx) => {
    const principal = requireWrite(ctx);
    const current = entries.get(ctx.params.id);
    const schema = schemas.get(current.type);
    await bus.emit('entry.beforePublish', { entry: current, principal });
    const published = workflow.publish(schema, current.id, { actor: principal.id });
    await bus.emit('entry.afterPublish', { entry: published, principal });
    json(ctx.res, 200, entryView(published, { includePublished: true }));
  });

  router.post('/entries/:id/unpublish', async (ctx) => {
    const principal = requireWrite(ctx);
    json(ctx.res, 200, entryView(workflow.unpublish(ctx.params.id, { actor: principal.id })));
  });

  router.get('/entries/:id/revisions', async (ctx) => {
    requireRead(ctx);
    const entry = entries.get(ctx.params.id);
    json(ctx.res, 200, { items: revisions.list('entries', entry.id) });
  });

  router.post('/entries/:id/revisions/:revision/restore', async (ctx) => {
    const principal = requireWrite(ctx);
    const restored = entries.restoreRevision(ctx.params.id, Number(ctx.params.revision));
    revisions.record({ collection: 'entries', id: restored.id, action: 'restore', after: restored, actor: principal.id });
    json(ctx.res, 200, entryView(restored, { includePublished: true }));
  });

  router.delete('/entries/:id/revisions', async (ctx) => {
    const principal = requireWrite(ctx);
    revisions.record({ collection: 'entries', id: ctx.params.id, action: 'revisions:cleared', actor: principal.id });
    noContent(ctx.res);
  });

  router.get('/entries/by-slug/:slug', async (ctx) => {
    requireRead(ctx);
    const schema = schemas.get(ctx.query.type ?? 'article');
    const entry = entries.find(schema, ctx.params.slug, ctx.query.locale ?? 'en');
    json(ctx.res, 200, entryView(entry, { includePublished: true }));
  });

  router.post('/entries/bulk', async (ctx) => {
    const principal = requireWrite(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    const schema = schemas.get(body.type);
    const result = entries.bulkCreate(schema, body.items ?? [], { locale: body.locale, actor: principal.id });
    json(ctx.res, 201, {
      created: entryListView(result.created),
      failed: result.failed,
      counts: entries.counts(),
    });
  });

  router.post('/entries/bulk/publish', async (ctx) => {
    const principal = requireWrite(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    const ids = Array.isArray(body.ids) ? body.ids : [];
    const result = entries.bulkStatus(ids, 'published', { actor: principal.id });
    json(ctx.res, 200, {
      changed: result.changed.map((entry) => entryView(entry)),
      skipped: result.skipped,
    });
  });

  router.get('/entries/:id/usage', async (ctx) => {
    requireRead(ctx);
    const entry = entries.get(ctx.params.id);
    const media = Object.entries(entry.values ?? {})
      .filter(([, value]) => typeof value === 'string' && value.includes('.'))
      .map(([field, value]) => ({ field, name: value, references: entries.usage(value) }));
    json(ctx.res, 200, { items: media });
  });

  router.post('/entries/:id/duplicate', async (ctx) => {
    const principal = requireWrite(ctx);
    const current = entries.get(ctx.params.id);
    const copy = entries.duplicate(current.id, { schema: schemas.get(current.type), actor: principal.id });
    json(ctx.res, 201, entryView(copy, { includePublished: true }));
  });
}
