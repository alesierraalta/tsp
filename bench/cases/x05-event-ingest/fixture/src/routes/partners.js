import { json, csvDownload } from '../http/response.js';
import { queryInt, queryList, readJson } from '../http/request.js';
import { requireAdmin, requireRead, requireWrite, tenantOf } from '../auth/principal.js';
import { PARTNER_FORMATS, blockedFields, bundleFiles, partnerRows, partnerSummary } from '../reports/partners.js';
import { assertTarget } from '../webhooks/targets.js';

export function partnerRoutes(router, services) {
  const { partners, intake, deliveries, audit } = services;

  const partnerOf = (ctx) => partners.require(ctx.params.id);

  router.get('/v1/partners', async (ctx) => {
    requireRead(ctx);
    const kind = ctx.query.kind;
    const items = partners.list().filter((partner) => kind === undefined || partner.kind === kind);
    json(ctx.res, 200, { items: items.map((partner) => partnerSummary(partner, [])), blocked: blockedFields() });
  });

  router.post('/v1/partners', async (ctx) => {
    requireAdmin(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    const partner = partners.define(body);
    json(ctx.res, 201, partnerSummary(partner, []));
  });

  router.get('/v1/partners/:id', async (ctx) => {
    requireRead(ctx);
    const partner = partnerOf(ctx);
    json(ctx.res, 200, { partner: partnerSummary(partner, []), files: partners.filesOf(partner.id) });
  });

  router.post('/v1/partners/:id/tenants', async (ctx) => {
    requireAdmin(ctx);
    const partner = partnerOf(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    const tenant = String(body.tenant ?? tenantOf(ctx));
    json(ctx.res, 200, partnerSummary(partners.assign(partner.id, tenant), []));
  });

  router.post('/v1/partners/:id/exports', async (ctx) => {
    const tenant = requireWrite(ctx).tenant;
    const partner = partnerOf(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes }).catch(() => ({}));
    const format = PARTNER_FORMATS.includes(body.format) ? body.format : 'csv';
    const events = intake.eventsOf(tenant, { name: body.name });
    const built = partners.export(partner.id, events, { format });
    audit?.record('partner.exported', { tenant, actor: ctx.principal.key, target: partner.id, details: { name: built.name } });
    json(ctx.res, 201, { name: built.name, manifest: built.manifest });
  });

  router.get('/v1/partners/:id/exports', async (ctx) => {
    requireRead(ctx);
    const partner = partnerOf(ctx);
    json(ctx.res, 200, { items: partners.filesOf(partner.id).slice(-queryInt(ctx.query, 'limit', 20)) });
  });

  router.get('/v1/partners/:id/exports/:name', async (ctx) => {
    requireRead(ctx);
    const partner = partnerOf(ctx);
    const file = partners.file(partner.id, ctx.params.name);
    if (!file) {
      json(ctx.res, 404, { error: 'not_found', message: `no export ${ctx.params.name} for ${partner.id}` });
      return;
    }
    if (file.format === 'csv') {
      csvDownload(ctx.res, file.text, file.name);
      return;
    }
    json(ctx.res, 200, { name: file.name, format: file.format, manifest: file.manifest, text: file.text });
  });

  router.post('/v1/partners/:id/bundles', async (ctx) => {
    const tenant = requireWrite(ctx).tenant;
    const partner = partnerOf(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes }).catch(() => ({}));
    const events = intake.eventsOf(tenant, { name: body.name });
    const bundle = bundleFiles(partner, partnerRows(partner, events), { format: body.format, size: body.size });
    json(ctx.res, 201, {
      partner: partner.id,
      rows: bundle.rows,
      bytes: bundle.bytes,
      checksum: bundle.checksum,
      files: bundle.files.map((file) => ({ name: file.name, rows: file.rows, checksum: file.checksum })),
    });
  });

  router.get('/v1/partners/:id/webhooks', async (ctx) => {
    requireRead(ctx);
    const partner = partnerOf(ctx);
    json(ctx.res, 200, { items: deliveries.list(partner.id, queryInt(ctx.query, 'limit', 20)) });
  });

  router.post('/v1/partners/:id/webhooks', async (ctx) => {
    requireAdmin(ctx);
    const partner = partnerOf(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    const target = assertTarget(body.url);
    const subscription = deliveries.subscribe(partner.id, target.url, queryList(ctx.query, 'event'));
    json(ctx.res, 201, { id: subscription.id, host: subscription.host, events: subscription.events });
  });

  router.delete('/v1/partners/:id/webhooks/:subscription', async (ctx) => {
    requireAdmin(ctx);
    const partner = partnerOf(ctx);
    const subscription = deliveries.list(partner.id).find((row) => row.subscription === ctx.params.subscription);
    if (!subscription) {
      json(ctx.res, 404, { error: 'not_found', message: `no subscription ${ctx.params.subscription}` });
      return;
    }
    json(ctx.res, 200, { removed: deliveries.unsubscribe(ctx.params.subscription) });
  });
}
