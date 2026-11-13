import { json } from '../http/response.js';
import { readJson, queryInt } from '../http/request.js';
import { requireAdmin, requireRead } from '../auth/principal.js';
import { describeScopes, SCOPES } from '../auth/keys.js';
import { describeRole, ROLE_CAPABILITIES } from '../auth/roles.js';
import { queuePressure, failedJobs } from '../jobs/queue.js';
import { eventSummary, topicRows } from '../store/events.js';
import { releaseSummary, deploymentSummary } from '../reports/summary.js';
import { metricsReport } from '../reports/metrics.js';
import { manifestFor, releaseRows, EXPORT_FORMATS } from '../reports/export.js';
import { redactedConfig } from '../config.js';
import { catalogReport } from '../domain/services.js';

export function adminRoutes(router, services) {
  const { keys, releases, deployments, incidents, events, queue, scheduler, services: domain, deliveryStats } = services;

  router.get('/admin/stats', async (ctx) => {
    requireRead(ctx);
    json(ctx.res, 200, {
      releases: releases.stats(),
      deployments: deployments.stats(),
      incidents: incidents.summary(),
      events: events.stats(),
      queue: queue.stats(),
      scheduler: scheduler.stats(),
      keys: keys.stats(),
    });
  });

  router.get('/admin/keys', async (ctx) => {
    requireAdmin(ctx);
    json(ctx.res, 200, { items: keys.list(ctx.query.tenant), scopes: describeScopes(SCOPES), stats: keys.stats() });
  });

  router.post('/admin/keys', async (ctx) => {
    requireAdmin(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    const issued = keys.issue(body.tenant ?? ctx.principal.tenant, { label: body.label, scopes: body.scopes });
    json(ctx.res, 201, { id: issued.id, tenant: issued.tenant, masked: issued.masked, scopes: issued.scopes, secret: issued.secret });
  });

  router.post('/admin/keys/:id/revoke', async (ctx) => {
    requireAdmin(ctx);
    json(ctx.res, 200, { revoked: keys.revoke(ctx.params.id) });
  });

  router.get('/admin/roles', async (ctx) => {
    requireRead(ctx);
    json(ctx.res, 200, { items: Object.keys(ROLE_CAPABILITIES).map((role) => describeRole(role)) });
  });

  router.get('/admin/events', async (ctx) => {
    requireRead(ctx);
    json(ctx.res, 200, {
      items: events.list({ tenant: ctx.query.tenant, topic: ctx.query.topic, limit: Math.max(1, Math.min(100, queryInt(ctx.query, 'limit', 25))) }),
      summary: eventSummary(events),
      topics: topicRows(events, ctx.query.tenant),
    });
  });

  router.get('/admin/queue', async (ctx) => {
    requireAdmin(ctx);
    json(ctx.res, 200, { items: queue.list(ctx.query.status), pressure: queuePressure(queue), failed: failedJobs(queue) });
  });

  router.post('/admin/queue', async (ctx) => {
    requireAdmin(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    json(ctx.res, 201, queue.enqueue(body.kind ?? 'digest', body.payload ?? {}, { priority: body.priority }));
  });

  router.post('/admin/queue/drain', async (ctx) => {
    requireAdmin(ctx);
    const done = await queue.drain(async (job) => ({ kind: job.kind, done: true }), { limit: queryInt(ctx.query, 'limit', 10) });
    json(ctx.res, 200, { items: done.map((job) => ({ id: job.id, kind: job.kind, status: job.status })) });
  });

  router.get('/admin/schedules', async (ctx) => {
    requireAdmin(ctx);
    json(ctx.res, 200, { items: scheduler.list() });
  });

  router.post('/admin/schedules/tick', async (ctx) => {
    requireAdmin(ctx);
    json(ctx.res, 200, scheduler.tick());
  });

  router.get('/admin/services', async (ctx) => {
    requireRead(ctx);
    json(ctx.res, 200, { items: domain.list(), stats: domain.stats(), report: catalogReport(domain) });
  });

  router.get('/admin/config', async (ctx) => {
    requireAdmin(ctx);
    json(ctx.res, 200, redactedConfig(services.config ?? {}));
  });

  router.get('/admin/metrics', async (ctx) => {
    requireRead(ctx);
    const report = metricsReport(deployments.list(), incidents.list());
    json(ctx.res, 200, { table: report.table, busiest: report.busiest, summary: { releases: releaseSummary(releases.list()), deployments: deploymentSummary(deployments.list()) } });
  });

  router.post('/admin/export', async (ctx) => {
    requireAdmin(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    const format = EXPORT_FORMATS.includes(body.format) ? body.format : 'csv';
    const rows = releaseRows(releases.list({ tenant: body.tenant }));
    json(ctx.res, 200, { manifest: manifestFor('releases', rows, format), deliveries: deliveryStats?.() ?? null });
  });

  router.get('/admin/health', async (ctx) => {
    requireRead(ctx);
    json(ctx.res, 200, { status: 'ok', queue: queue.stats(), events: events.size() });
  });
}

// The freeze calendar and the policy book answer the two questions a promotion asks before it runs: is the
// environment frozen, and does this actor have the role the environment's policy asks for.
export function freezeRoutes(router, services) {
  const { freezes, policies, environments } = services;

  router.get('/admin/freezes', async (ctx) => {
    requireRead(ctx);
    json(ctx.res, 200, {
      items: freezes.list(ctx.query.environment).map((freeze) => ({ id: freeze.id, environment: freeze.environment, from: freeze.from, to: freeze.to, reason: freeze.reason })),
      stats: freezes.stats(),
      upcoming: freezes.upcoming(ctx.query.environment ?? 'production').length,
    });
  });

  router.post('/admin/freezes', async (ctx) => {
    requireAdmin(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    const freeze = freezes.add({ ...body, created_by: ctx.principal.id });
    json(ctx.res, 201, { id: freeze.id, environment: freeze.environment, from: freeze.from, to: freeze.to });
  });

  router.delete('/admin/freezes/:id', async (ctx) => {
    requireAdmin(ctx);
    json(ctx.res, 200, { removed: freezes.remove(ctx.params.id) });
  });

  router.get('/admin/policies', async (ctx) => {
    requireRead(ctx);
    json(ctx.res, 200, { items: policies.list(), stats: policies.stats(), matrix: policies.matrix(environments.names()) });
  });

  router.post('/admin/policies', async (ctx) => {
    requireAdmin(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    json(ctx.res, 201, policies.define(body));
  });

  router.post('/admin/policies/check', async (ctx) => {
    requireRead(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    json(ctx.res, 200, policies.check(body.kind ?? 'deploy', body.environment ?? 'production', { role: body.role, approvals: body.approvals, freeze: body.freeze }));
  });
}
