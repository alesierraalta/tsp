import { json, noContent, send } from '../http/response.js';
import { readBody, queryInt } from '../http/request.js';
import { requireRead, requireWrite } from '../auth/principal.js';
import { mediaView } from '../http/serialize.js';
import { contentTypeOf } from '../http/headers.js';
import { describeMedia } from '../media/metadata.js';

export function mediaRoutes(router, services) {
  const { media, bus, planVariantsCache } = services;

  router.get('/media', async (ctx) => {
    requireRead(ctx);
    const items = await media.list();
    json(ctx.res, 200, { items });
  });

  router.get('/media/:name', async (ctx) => {
    requireRead(ctx);
    const described = await media.describe(ctx.params.name);
    const body = await media.read(ctx.params.name);
    if (ctx.query.raw === '1') {
      send(ctx.res, 200, body, { 'content-type': contentTypeOf({ 'content-type': described.type }) });
      return;
    }
    json(ctx.res, 200, mediaView(described));
  });

  router.put('/media/:name', async (ctx) => {
    const principal = requireWrite(ctx);
    const body = await readBody(ctx.req, { limit: media.maxBytes });
    const described = await media.put(ctx.params.name, body);
    await bus.emit('media.afterUpload', { media: described, principal });
    json(ctx.res, 201, mediaView({ ...described, image: described.type.startsWith('image/') }));
  });

  router.delete('/media/:name', async (ctx) => {
    requireWrite(ctx);
    json(ctx.res, 200, await media.remove(ctx.params.name));
  });

  router.get('/media/:name/variants', async (ctx) => {
    requireRead(ctx);
    const described = await media.describe(ctx.params.name);
    const body = await media.read(ctx.params.name);
    const wanted = String(ctx.query.presets ?? 'thumb,card').split(',');
    json(ctx.res, 200, {
      name: described.name,
      variants: planVariantsCache(ctx.params.name, body, wanted),
    });
  });

  router.get('/media/:name/metadata', async (ctx) => {
    requireRead(ctx);
    const body = await media.read(ctx.params.name);
    json(ctx.res, 200, describeMedia(ctx.params.name, body, {}));
  });

  router.get('/media', async (ctx) => {
    requireRead(ctx);
    const extension = ctx.query.extension;
    if (!extension) return;
    const items = await media.listByExtension(extension);
    json(ctx.res, 200, { items, limit: queryInt(ctx.query, 'limit', 50) });
  });

  router.delete('/media', async (ctx) => {
    requireWrite(ctx);
    noContent(ctx.res);
  });
}
