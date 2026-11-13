import { inflateSync } from 'node:zlib';
import { unsafePath } from '../http/errors.js';
import { readJson } from '../http/request.js';
import { json, text } from '../http/response.js';
import { assertCan, principalFromHeaders } from '../auth/principal.js';
import { assertPlainName, isArchiveName } from '../files/paths.js';
import { tenantReport, userReport, usageReport, csvRows, summaryLine } from '../reports/index.js';

export function reportRoutes(router, options) {
  const { catalog, archive, documents, store, secret, clock } = options;
  const principalOf = (ctx) => principalFromHeaders(ctx.headers, secret, { now: clock().getTime() });

  router.get('/reports', async (ctx) => {
    const principal = principalOf(ctx);
    assertCan(principal, 'read', ctx.params.id ?? principal.tenantId);
    const [listed, entries] = await Promise.all([catalog.index(), catalog.entries()]);
    json(ctx.res, 200, { listed, entries });
  });

  router.get('/reports/:name', async (ctx) => {
    const principal = principalOf(ctx);
    assertCan(principal, 'read', principal.tenantId);
    const report = await catalog.read(assertPlainName(ctx.params.name));
    text(ctx.res, 200, report.body);
  });

  router.post('/reports/:name', async (ctx) => {
    const principal = principalOf(ctx);
    assertCan(principal, 'write', principal.tenantId);
    const body = await readJson(ctx.req);
    const written = await catalog.write(
      assertPlainName(ctx.params.name),
      buildReport(ctx.params.name, body, store),
    );
    json(ctx.res, 201, written);
  });

  router.get('/reports/archive/list', async (ctx) => {
    principalOf(ctx);
    json(ctx.res, 200, { entries: await archive.entries() });
  });

  router.get('/reports/archive/:name', async (ctx) => {
    const principal = principalOf(ctx);
    assertCan(principal, 'read', principal.tenantId);
    const entry = await archive.read(ctx.params.name);
    json(ctx.res, 200, entry);
  });

  router.put('/reports/archive/:name', async (ctx) => {
    const principal = principalOf(ctx);
    assertCan(principal, 'admin', principal.tenantId);
    if (!isArchiveName(ctx.params.name)) {
      throw unsafePath(ctx.params.name);
    }
    const body = await readJson(ctx.req);
    const written = await archive.write(ctx.params.name, body.rows ?? []);
    json(ctx.res, 201, written);
  });

  router.get('/reports/export/:name', async (ctx) => {
    const principal = principalOf(ctx);
    assertCan(principal, 'read', principal.tenantId);
    const entry = await archive.read(ctx.params.name);
    const fields = Object.keys(entry.rows?.[0] ?? {});
    const body = csvRows(entry.rows ?? [], fields);
    text(ctx.res, 200, body, { 'content-type': 'text/csv; charset=utf-8' });
  });

  router.get('/documents', async (ctx) => {
    principalOf(ctx);
    json(ctx.res, 200, { rows: await documents.list(), usage: await documents.usage() });
  });

  router.get('/documents/:name', async (ctx) => {
    const principal = principalOf(ctx);
    assertCan(principal, 'read', principal.tenantId);
    const document = await documents.read(ctx.params.name);
    text(ctx.res, 200, document.data, { 'content-type': document.type });
  });

  router.put('/documents/:name', async (ctx) => {
    const principal = principalOf(ctx);
    assertCan(principal, 'write', principal.tenantId);
    const body = await readJson(ctx.req);
    const written = await documents.write(ctx.params.name, String(body.text ?? ''));
    json(ctx.res, 201, written);
  });

  router.delete('/documents/:name', async (ctx) => {
    const principal = principalOf(ctx);
    assertCan(principal, 'admin', principal.tenantId);
    json(ctx.res, 200, await documents.remove(ctx.params.name));
  });

  router.post('/documents/:name/decompress', async (ctx) => {
    const principal = principalOf(ctx);
    assertCan(principal, 'write', principal.tenantId);
    const document = await documents.read(ctx.params.name);
    const inflated = inflateSync(document.data);
    json(ctx.res, 200, { bytes: inflated.length, preview: inflated.toString('utf8').slice(0, 200) });
  });
}

export function buildReport(name, body, store) {
  if (name === 'tenants') {
    return tenantReport(store.all('tenants'), store.all('users'), { day: body.day });
  }
  if (name === 'users') {
    const rows = userReport(store.all('users'));
    return `# Users\n\n${csvRows(rows, ['email', 'tenant', 'roles', 'active'])}\n`;
  }
  if (name === 'usage') {
    const rows = usageReport(store.all('audits'), { limit: body.limit });
    return `# Usage\n\n${summaryLine(rows)}\n`;
  }
  return `# ${name}\n\n${summaryLine(store.all('audits'))}\n`;
}
