import { json, created } from '../http/response.js';
import { readJson, pageParams, queryInt } from '../http/request.js';
import { requireRead, requireWrite } from '../auth/principal.js';
import { stageOf, progressOf, elapsedMinutes, isFinal } from '../domain/deployments.js';
import { deploymentPayload } from '../webhooks/payloads.js';
import { deploymentSummary } from '../reports/summary.js';
import { describeRollout, rolloutPlan } from '../domain/rollouts.js';
import { approvalsRequired } from '../domain/environments.js';

export function deploymentRoutes(router, services) {
  const { releases, deployments, approvals, queue, policies, freezes } = services;

  const describe = (deployment) => ({
    id: deployment.id,
    release: deployment.release,
    service: deployment.service,
    version: deployment.version,
    environment: deployment.environment,
    stage: stageOf(deployment),
    status: deployment.status,
    progress: progressOf(deployment),
    elapsed_minutes: elapsedMinutes(deployment),
    approvals: deployment.approvals.length,
  });

  router.get('/deployments', async (ctx) => {
    const principal = requireRead(ctx);
    const items = deployments.list({
      tenant: principal.tenant,
      service: ctx.query.service,
      environment: ctx.query.environment,
      status: ctx.query.status,
    });
    const page = pageParams(ctx.query);
    json(ctx.res, 200, {
      items: items.slice(page.offset, page.offset + page.limit).map(describe),
      total: items.length,
      summary: deploymentSummary(items, { tenant: principal.tenant }),
      open: deployments.open().length,
    });
  });

  router.post('/deployments', async (ctx) => {
    const principal = requireWrite(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    const release = releases.require(body.release);
    const decision = policies?.check('deploy', release.environment, { role: body.role ?? 'releaser', channel: release.channel });
    if (decision && decision.allowed === false) {
      json(ctx.res, 403, { error: 'policy_refused', message: decision.reasons[0] ?? 'policy refused', reasons: decision.reasons });
      return;
    }
    const instant = new Date(services.clock());
    if (freezes?.blocks(release.environment, instant) && body.override !== true) {
      const active = freezes.active(release.environment, instant);
      json(ctx.res, 409, { error: 'change_freeze', message: `${release.environment} is frozen until ${active.to}`, reason: active.reason });
      return;
    }
    const deployment = deployments.start(release, { actor: principal.id });
    queue?.enqueue('notify', deploymentPayload(deployment, { tenant: deployment.tenant }), { priority: 2 });
    created(ctx.res, describe(deployment));
  });

  router.get('/deployments/:id', async (ctx) => {
    requireRead(ctx);
    const deployment = deployments.require(ctx.params.id);
    json(ctx.res, 200, {
      deployment: describe(deployment),
      plan: deployment.plan.map((stage) => ({ name: stage.name, approvals: stage.approvals, manual: stage.manual })),
      approvals: approvals.list(deployment.id).map((approval) => ({ id: approval.id, state: approval.state, by: approval.by })),
    });
  });

  router.post('/deployments/:id/advance', async (ctx) => {
    requireWrite(ctx);
    const deployment = deployments.advance(ctx.params.id, { actor: ctx.principal.key });
    json(ctx.res, 200, describe(deployment));
  });

  router.post('/deployments/:id/approve', async (ctx) => {
    requireWrite(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    const deployment = deployments.require(ctx.params.id);
    const pending = approvals.pending(deployment.id);
    if (pending.length === 0) {
      const required = approvalsRequired(deployment.environment);
      const requested = approvals.request(deployment, { by: ctx.principal.key, role: body.role ?? 'approver', note: body.note });
      json(ctx.res, 202, { state: 'requested', approval: requested.id, required });
      return;
    }
    const granted = approvals.grant(pending[0].id, { note: body.note });
    const next = deployments.approve(deployment.id, { by: granted.by, role: granted.role, note: granted.note, at: granted.decided_at });
    json(ctx.res, 200, { approval: granted.id, deployment: describe(next) });
  });

  router.post('/deployments/:id/refuse', async (ctx) => {
    requireWrite(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    const deployment = deployments.require(ctx.params.id);
    const pending = approvals.pending(deployment.id);
    if (pending.length === 0) {
      json(ctx.res, 409, { error: 'nothing_to_refuse', message: `deployment ${deployment.id} waits for no approval` });
      return;
    }
    const refused = approvals.refuse(pending[0].id, { reason: body.reason });
    const failed = deployments.finish(deployment.id, 'failed', { reason: `approval refused: ${refused.note}` });
    json(ctx.res, 200, { approval: refused.id, deployment: describe(failed) });
  });

  router.post('/deployments/:id/finish', async (ctx) => {
    requireWrite(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    const deployment = deployments.require(ctx.params.id);
    if (isFinal(deployment.status)) {
      json(ctx.res, 409, { error: 'deployment_finished', message: `deployment ${deployment.id} is ${deployment.status}` });
      return;
    }
    const finished = deployments.finish(deployment.id, body.status === 'failed' ? 'failed' : 'succeeded', { actor: ctx.principal.key, reason: body.reason });
    queue?.enqueue('notify', deploymentPayload(finished, { tenant: finished.tenant, event: 'deployment.finished' }), { priority: 1 });
    json(ctx.res, 200, describe(finished));
  });

  router.post('/deployments/:id/rollback', async (ctx) => {
    requireWrite(ctx);
    const body = await readJson(ctx.req, { limit: services.maxBodyBytes });
    const rolled = deployments.rollback(ctx.params.id, { actor: ctx.principal.key, reason: body.reason });
    queue?.enqueue('notify', deploymentPayload(rolled, { tenant: rolled.tenant }), { priority: 3 });
    json(ctx.res, 200, describe(rolled));
  });

  router.get('/deployments/:id/rollout', async (ctx) => {
    requireRead(ctx);
    const deployment = deployments.require(ctx.params.id);
    const plan = rolloutPlan(deployment.environment);
    json(ctx.res, 200, describeRollout(plan, queryInt(ctx.query, 'step', 0)));
  });
}
