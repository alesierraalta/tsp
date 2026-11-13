import { json } from '../http/response.js';
import { queryInt, readJson } from '../http/request.js';
import { requireRead, requireWrite } from '../auth/principal.js';
import { highlight } from '../search/index.js';

export function searchRoutes(router, services) {
  const { search, schemas, entries } = services;

  router.get('/search', async (ctx) => {
    requireRead(ctx);
    const result = search.search(ctx.query.q ?? '', {
      limit: queryInt(ctx.query, 'limit', 20),
      type: ctx.query.type,
      locale: ctx.query.locale,
    });
    json(ctx.res, 200, {
      total: result.total,
      items: result.hits.map((hit) => ({ ...hit, preview: highlight(hit.snippet, ctx.query.q ?? '') })),
    });
  });

  router.get('/search/suggest', async (ctx) => {
    requireRead(ctx);
    json(ctx.res, 200, { items: search.suggestions(ctx.query.prefix ?? '', queryInt(ctx.query, 'limit', 8)) });
  });

  router.post('/search/reindex', async (ctx) => {
    requireWrite(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    const schema = body.type ? schemas.get(body.type) : undefined;
    const records = schema ? entries.list(schema) : services.store.all('entries');
    const documents = search.rebuild(records);
    json(ctx.res, 200, { documents });
  });

  router.get('/search/stats', async (ctx) => {
    requireRead(ctx);
    json(ctx.res, 200, search.stats());
  });
}
