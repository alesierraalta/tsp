import { json } from '../http/response.js';
import { readJson } from '../http/request.js';
import { requireAdmin, requireRead, requireWrite } from '../auth/principal.js';
import { pluginView } from '../http/serialize.js';
import { manifestSummary, parseManifest, validateManifestUpdate } from '../plugins/manifest.js';

export function pluginRoutes(router, services) {
  const { plugins, pluginAssets, registry, schemas } = services;

  router.get('/plugins', async (ctx) => {
    requireRead(ctx);
    json(ctx.res, 200, { items: registry.list() });
  });

  router.get('/plugins/discovered', async (ctx) => {
    requireRead(ctx);
    const found = await plugins.discover();
    json(ctx.res, 200, { items: found.map(manifestSummary), errors: plugins.errors });
  });

  router.post('/plugins/load', async (ctx) => {
    requireAdmin(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    if (!body.manifest) {
      const discovered = await plugins.load(body.name);
      json(ctx.res, 201, pluginView(discovered));
      return;
    }
    const manifest = parseManifest(body.manifest);
    registry.register(manifest);
    json(ctx.res, 201, pluginView(manifest));
  });

  router.patch('/plugins/:name', async (ctx) => {
    requireAdmin(ctx);
    const current = registry.get(ctx.params.name);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    validateManifestUpdate(current, body);
    const next = registry.replace({ ...current, ...body });
    json(ctx.res, 200, pluginView(next));
  });

  router.post('/plugins/:name/enable', async (ctx) => {
    requireAdmin(ctx);
    json(ctx.res, 200, pluginView(plugins.enable(ctx.params.name)));
  });

  router.post('/plugins/:name/disable', async (ctx) => {
    requireAdmin(ctx);
    json(ctx.res, 200, pluginView(plugins.disable(ctx.params.name)));
  });

  router.post('/plugins/:name/transform', async (ctx) => {
    const principal = requireWrite(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    const schema = schemas.get(body.type ?? 'article');
    const entry = body.entry_id
      ? services.entries.get(body.entry_id)
      : services.entries.find(schema, body.slug ?? 'untitled');
    const result = await plugins.transform(ctx.params.name, entry);
    services.logger.info('plugin transform', { plugin: ctx.params.name, entry: entry.slug, actor: principal.id });
    json(ctx.res, 200, result);
  });

  router.get('/plugins/assets', async (ctx) => {
    requireRead(ctx);
    json(ctx.res, 200, { items: await pluginAssets.list() });
  });

  router.put('/plugins/assets/:name', async (ctx) => {
    requireAdmin(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    json(ctx.res, 201, await pluginAssets.write(ctx.params.name, body.body ?? ''));
  });

  router.get('/plugins/assets/:name', async (ctx) => {
    requireRead(ctx);
    const asset = await pluginAssets.read(ctx.params.name);
    json(ctx.res, 200, { name: asset.name, type: asset.type, bytes: asset.body.length, body: asset.body.toString('utf8') });
  });
}
