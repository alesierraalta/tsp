import { json, created } from '../http/response.js';
import { readJson, pageParams, queryList } from '../http/request.js';
import { requireRead, requireWrite } from '../auth/principal.js';
import { describeRelease } from '../domain/releases.js';
import { releasePayload } from '../webhooks/payloads.js';
import { releaseSummary } from '../reports/summary.js';
import { releaseRows } from '../reports/export.js';
import { releaseNotes, notesMarkdown } from '../reports/notes.js';
import { releasePage, releasesPage } from '../views/releases.js';

export function releaseRoutes(router, services) {
  const { releases, deployments, incidents, queue } = services;

  const notify = (release) => {
    queue?.enqueue('notify', releasePayload(release, { tenant: release.tenant }), { priority: 2 });
  };

  router.get('/releases', async (ctx) => {
    const principal = requireRead(ctx);
    const items = releases.list({
      tenant: principal.tenant,
      service: ctx.query.service,
      environment: ctx.query.environment,
      status: ctx.query.status,
      channel: ctx.query.channel,
    });
    if (ctx.wantsHtml) {
      json(ctx.res, 200, { html: releasesPage(items, { tenant: principal.tenant }), rows: items.length });
      return;
    }
    const page = pageParams(ctx.query);
    json(ctx.res, 200, {
      items: items.slice(page.offset, page.offset + page.limit).map(describeRelease),
      total: items.length,
      summary: releaseSummary(items, { tenant: principal.tenant }),
    });
  });

  router.post('/releases', async (ctx) => {
    const principal = requireWrite(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    const release = releases.create({ ...body, tenant: principal.tenant, created_by: principal.key });
    notify(release);
    created(ctx.res, describeRelease(release));
  });

  router.get('/releases/:id', async (ctx) => {
    requireRead(ctx);
    const release = releases.require(ctx.params.id);
    const related = deployments.list({ service: release.service, environment: release.environment });
    if (ctx.wantsHtml) {
      json(ctx.res, 200, { html: releasePage(release, { deployments: related, tenant: release.tenant }) });
      return;
    }
    json(ctx.res, 200, { release: describeRelease(release), deployments: related.map((deployment) => deployment.id) });
  });

  router.post('/releases/:id/promote', async (ctx) => {
    requireWrite(ctx);
    const release = releases.promote(ctx.params.id, { actor: ctx.principal.key });
    notify(release);
    json(ctx.res, 200, describeRelease(release));
  });

  router.post('/releases/:id/cancel', async (ctx) => {
    requireWrite(ctx);
    json(ctx.res, 200, describeRelease(releases.cancel(ctx.params.id, { actor: ctx.principal.key })));
  });

  router.post('/releases/:id/status', async (ctx) => {
    requireWrite(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    json(ctx.res, 200, describeRelease(releases.transition(ctx.params.id, String(body.status ?? ''), { actor: ctx.principal.key })));
  });

  router.get('/releases/:id/notes', async (ctx) => {
    requireRead(ctx);
    const release = releases.require(ctx.params.id);
    const notes = releaseNotes(release);
    json(ctx.res, 200, { notes, markdown: notesMarkdown(notes) });
  });

  router.get('/releases/:id/timeline', async (ctx) => {
    requireRead(ctx);
    const release = releases.require(ctx.params.id);
    json(ctx.res, 200, {
      release: release.id,
      deployments: deployments.list({ service: release.service }).map((deployment) => ({ id: deployment.id, status: deployment.status, stage: deployment.stage })),
      incidents: incidents.list({ service: release.service }).map((incident) => ({ id: incident.id, severity: incident.severity, state: incident.state })),
    });
  });

  router.get('/releases/export', async (ctx) => {
    const principal = requireRead(ctx);
    const formats = queryList(ctx.query, 'format');
    const items = releases.list({ tenant: principal.tenant });
    json(ctx.res, 200, {
      rows: releaseRows(items).length,
      format: formats[0] ?? 'csv',
      services: [...new Set(items.map((release) => release.service))].sort(),
    });
  });
}

export function channelRoutes(router, services) {
  const { releases } = services;

  router.get('/releases/channels', async (ctx) => {
    requireRead(ctx);
    const items = releases.list({ tenant: ctx.principal.tenant });
    const channels = {};
    for (const release of items) channels[release.channel] = (channels[release.channel] ?? 0) + 1;
    json(ctx.res, 200, { channels, total: items.length });
  });

  router.get('/releases/:id/changes', async (ctx) => {
    requireRead(ctx);
    const release = releases.require(ctx.params.id);
    const kinds = {};
    for (const entry of release.changelog) {
      const kind = typeof entry === 'string' ? 'chore' : entry.kind ?? 'chore';
      kinds[kind] = (kinds[kind] ?? 0) + 1;
    }
    json(ctx.res, 200, { release: release.id, entries: release.changelog.length, kinds });
  });

  router.patch('/releases/:id/summary', async (ctx) => {
    requireWrite(ctx);
    const release = releases.require(ctx.params.id);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    const next = { ...release, summary: String(body.summary ?? release.summary), updated_at: new Date().toISOString() };
    releases.releases.set(release.id, next);
    json(ctx.res, 200, describeRelease(next));
  });
}
