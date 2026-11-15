import { json, created } from '../http/response.js';
import { readJson, pageParams } from '../http/request.js';
import { requireRead, requireWrite } from '../auth/principal.js';
import { severityRank, isBlocking } from '../domain/incidents.js';
import { incidentPayload } from '../webhooks/payloads.js';

export function incidentRoutes(router, services) {
  const { incidents, releases, deployments, queue } = services;

  const describe = (incident) => ({
    id: incident.id,
    title: incident.title,
    severity: incident.severity,
    service: incident.service,
    environment: incident.environment,
    state: incident.state,
    owner: incident.owner,
    opened_at: incident.opened_at,
    resolved_at: incident.resolved_at,
  });

  router.get('/incidents', async (ctx) => {
    const principal = requireRead(ctx);
    const items = incidents.list({
      tenant: principal.tenant,
      state: ctx.query.state,
      service: ctx.query.service,
    });
    const page = pageParams(ctx.query);
    json(ctx.res, 200, {
      items: items.slice(page.offset, page.offset + page.limit).map(describe),
      total: items.length,
      summary: incidents.summary(),
      blocking: incidents.blocking().length,
    });
  });

  router.post('/incidents', async (ctx) => {
    const principal = requireWrite(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    const incident = incidents.open({ ...body, tenant: principal.tenant, opened_by: principal.key });
    queue?.enqueue('notify', incidentPayload(incident, { tenant: incident.tenant }), { priority: 3 });
    created(ctx.res, describe(incident));
  });

  router.get('/incidents/:id', async (ctx) => {
    requireRead(ctx);
    const incident = incidents.require(ctx.params.id);
    json(ctx.res, 200, { incident: describe(incident), blocking: isBlocking(incident.severity), rank: severityRank(incident.severity) });
  });

  router.post('/incidents/:id/assign', async (ctx) => {
    requireWrite(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    json(ctx.res, 200, describe(incidents.assign(ctx.params.id, body.owner ?? ctx.principal.key)));
  });

  router.post('/incidents/:id/resolve', async (ctx) => {
    requireWrite(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    json(ctx.res, 200, describe(incidents.resolve(ctx.params.id, { resolution: body.resolution })));
  });

  router.get('/incidents/:id/context', async (ctx) => {
    requireRead(ctx);
    const incident = incidents.require(ctx.params.id);
    json(ctx.res, 200, {
      incident: describe(incident),
      releases: releases.list({ service: incident.service }).slice(0, 5).map((release) => ({ id: release.id, version: release.version, status: release.status })),
      deployments: deployments.list({ service: incident.service }).slice(0, 5).map((deployment) => ({ id: deployment.id, status: deployment.status, stage: deployment.stage })),
    });
  });
}
