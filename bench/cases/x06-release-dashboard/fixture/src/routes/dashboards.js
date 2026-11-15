import { json } from '../http/response.js';
import { queryInt, queryList } from '../http/request.js';
import { requireRead } from '../auth/principal.js';
import { dashboardPage, dashboardStats, activityRows } from '../views/dashboard.js';
import { combinedSummary } from '../reports/summary.js';
import { buildTimeline, timelineRows, countKinds } from '../reports/timeline.js';
import { metricsReport, metricTable } from '../reports/metrics.js';
import { releaseSummary } from '../reports/summary.js';
import { environmentTable } from '../domain/environments.js';

export function dashboardRoutes(router, services) {
  const { releases, deployments, incidents, deliveries, approvals, environments } = services;

  router.get('/', async (ctx) => {
    const principal = requireRead(ctx);
    const releasesOfTenant = releases.list({ tenant: principal.tenant });
    const deploymentsOfTenant = deployments.list({ tenant: principal.tenant });
    const incidentsOfTenant = incidents.list({ tenant: principal.tenant });
    const view = {
      title: 'Release dashboard',
      tenant: principal.tenant,
      environments: environments.names(),
      environmentLabels: environments.labels(),
      environment: ctx.query.environment,
      summary: dashboardStats(deploymentsOfTenant, releasesOfTenant, incidentsOfTenant),
      byEnvironment: releaseSummary(releasesOfTenant).byEnvironment,
      deployments: deploymentsOfTenant.slice(0, 10),
      incidents: incidentsOfTenant.filter((incident) => incident.state === 'open'),
      current: deploymentsOfTenant.find((deployment) => !['succeeded', 'failed', 'rolled_back'].includes(deployment.status)),
      deliveries: deliveries.list({ tenant: principal.tenant, limit: 10 }),
      hosts: deliveries.byHost(),
    };
    if (ctx.wantsHtml) {
      json(ctx.res, 200, { html: dashboardPage(view) });
      return;
    }
    json(ctx.res, 200, {
      tenant: principal.tenant,
      summary: view.summary,
      environments: environments.table(),
      deployments: view.deployments.length,
    });
  });

  router.get('/dashboards/activity', async (ctx) => {
    const principal = requireRead(ctx);
    const days = Math.max(1, Math.min(90, queryInt(ctx.query, 'days', 14)));
    const rows = activityRows(deployments.list({ tenant: principal.tenant })).slice(-days);
    json(ctx.res, 200, { days: rows.length, rows: rows.map((row) => ({ day: row.day, events: row.events })) });
  });

  router.get('/dashboards/timeline', async (ctx) => {
    const principal = requireRead(ctx);
    const sources = {
      releases: releases.list({ tenant: principal.tenant }),
      deployments: deployments.list({ tenant: principal.tenant }),
      incidents: incidents.list({ tenant: principal.tenant }),
      approvals: approvals.list(),
    };
    const entries = buildTimeline(sources);
    json(ctx.res, 200, { entries: entries.length, kinds: countKinds(entries), rows: timelineRows(entries).slice(0, Math.max(1, Math.min(100, queryInt(ctx.query, 'limit', 25)))) });
  });

  router.get('/dashboards/metrics', async (ctx) => {
    const principal = requireRead(ctx);
    const report = metricsReport(deployments.list({ tenant: principal.tenant }), incidents.list({ tenant: principal.tenant }));
    json(ctx.res, 200, { table: metricTable(deployments.list({ tenant: principal.tenant }), incidents.list({ tenant: principal.tenant })), lead_time: report.lead_time, failure: report.failure });
  });

  router.get('/dashboards/summary', async (ctx) => {
    const principal = requireRead(ctx);
    json(ctx.res, 200, combinedSummary(releases.list({ tenant: principal.tenant }), deployments.list({ tenant: principal.tenant })));
  });

  router.get('/dashboards/environments', async (ctx) => {
    requireRead(ctx);
    json(ctx.res, 200, { items: environmentTable() });
  });

  router.get('/dashboards/services', async (ctx) => {
    requireRead(ctx);
    const services = queryList(ctx.query, 'service');
    json(ctx.res, 200, { items: services.length === 0 ? [] : services, table: environmentTable() });
  });
}

export function createEnvironmentRegistry(options = {}) {
  const labels = { ...(options.labels ?? {}) };
  return {
    names: () => environmentTable().map((entry) => entry.name),
    labels: () => ({ ...labels }),
    labelFor: (name) => labels[name] ?? name,
    setLabel: (name, label) => {
      labels[name] = String(label);
      return { ...labels };
    },
    table: () => environmentTable(),
  };
}

export function readinessRoutes(router, services) {
  const { releases, deployments, incidents, approvals, policies, freezes, catalog } = services;

  router.get('/dashboards/readiness', async (ctx) => {
    const principal = requireRead(ctx);
    const releasesOfTenant = releases.list({ tenant: principal.tenant });
    const deploymentsOfTenant = deployments.list({ tenant: principal.tenant });
    const tenantsIncidents = incidents.list({ tenant: principal.tenant, state: 'open' });
    const rows = catalog.list().map((service) => {
      const scoped = deploymentsOfTenant.filter((deployment) => deployment.service === service.id);
      const open = tenantsIncidents.filter((incident) => incident.service === service.id);
      return {
        service: service.id,
        deployments: scoped.length,
        blocking: open.filter((incident) => ['sev1', 'sev2'].includes(incident.severity)).length,
        incidents: open.length,
      };
    });
    json(ctx.res, 200, {
      rows,
      approvals: approvals.pending().length,
      frozen: freezes.active('production') !== undefined,
      policies: policies.stats().policies,
      releases: releasesOfTenant.length,
    });
  });
}
