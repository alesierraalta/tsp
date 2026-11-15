import { json, send } from '../http/response.js';
import { readJson } from '../http/request.js';
import { requireRead, requireWrite } from '../auth/principal.js';
import { FORMATS } from '../export/exporters.js';
import { detectFormat, recordsFrom } from '../export/importer.js';

export function exportRoutes(router, services) {
  const { exporters, importer, schemas, bus } = services;

  router.get('/exports', async (ctx) => {
    requireRead(ctx);
    json(ctx.res, 200, { items: await exporters.list() });
  });

  router.get('/exports/:name', async (ctx) => {
    requireRead(ctx);
    const body = await exporters.read(ctx.params.name);
    send(ctx.res, 200, body, { 'content-type': 'text/plain; charset=utf-8' });
  });

  router.post('/exports', async (ctx) => {
    const principal = requireWrite(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    const format = body.format ?? 'json';
    if (!FORMATS.includes(format)) {
      json(ctx.res, 422, { error: 'unsupported_format', message: `${format} is not an export format` });
      return;
    }
    const result = await exporters.exportType(body.type, format, {
      locale: body.locale,
      status: body.status,
      name: body.name,
    });
    services.logger.info('export written', { name: result.name, rows: result.rows, actor: principal.id });
    json(ctx.res, 201, result);
  });

  router.post('/imports', async (ctx) => {
    const principal = requireWrite(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    const schema = schemas.get(body.type);
    const format = body.format ?? detectFormat(body.name ?? 'import.json');
    const rows = recordsFrom(body.content ?? '', format);
    const result = importer.importRecords(schema, rows, {
      locale: body.locale,
      status: body.status,
      lenient: body.lenient === true,
      actor: principal.id,
    });
    await bus.emit('export.afterWrite', { imported: result.created, type: schema.api_id });
    json(ctx.res, 201, { created: result.created, skipped: result.skipped, summary: importer.summary() });
  });

  router.post('/imports/validate', async (ctx) => {
    requireWrite(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    const schema = schemas.get(body.type);
    const format = body.format ?? detectFormat(body.name ?? 'import.json');
    const rows = recordsFrom(body.content ?? '', format);
    json(ctx.res, 200, { problems: importer.validateOnly(schema, rows) });
  });
}
