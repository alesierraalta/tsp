import { badRequest, conflict } from '../util/errors.js';
import { newId } from '../util/ids.js';
import { nowIso, diffMinutes } from '../util/time.js';
import { environmentOf } from './environments.js';
import { stagePlan, nextStage } from './stages.js';

export const DEPLOYMENT_STATUSES = ['pending', 'awaiting_approval', 'in_progress', 'succeeded', 'failed', 'rolled_back'];

export const FINAL_STATUSES = ['succeeded', 'failed', 'rolled_back'];

export function isFinal(status) {
  return FINAL_STATUSES.includes(String(status));
}

export function normalizeDeployment(release, options = {}) {
  if (!release?.id) throw badRequest('invalid_deployment', 'a deployment needs a release');
  const plan = stagePlan(release.environment, options);
  return {
    id: options.id ?? newId('dep'),
    tenant: release.tenant,
    release: release.id,
    version: release.version,
    service: release.service,
    environment: environmentOf(release.environment),
    plan,
    stage: plan[0].name,
    stage_index: 0,
    status: 'pending',
    started_by: String(options.actor ?? 'system'),
    started_at: nowIso(options.clock),
    updated_at: null,
    finished_at: null,
    failure: null,
    rollback_of: options.rollback_of ?? null,
    approvals: [],
  };
}

export function stageOf(deployment) {
  return deployment.plan[deployment.stage_index]?.name ?? deployment.stage;
}

export function progressOf(deployment) {
  const total = Math.max(1, deployment.plan.length);
  return Math.round(((deployment.stage_index + 1) / total) * 100);
}

export function elapsedMinutes(deployment, clock = () => new Date()) {
  return diffMinutes(deployment.started_at, deployment.finished_at ?? new Date(clock()));
}

export class DeploymentBook {
  constructor(options = {}) {
    this.clock = options.clock ?? (() => new Date());
    this.events = options.events;
    this.deployments = new Map();
  }

  start(release, options = {}) {
    const deployment = normalizeDeployment(release, { clock: this.clock, ...options });
    const active = this.activeFor(deployment.service, deployment.environment);
    if (active) throw conflict('deployment_active', `${deployment.service} is already deploying to ${deployment.environment}`);
    this.deployments.set(deployment.id, deployment);
    this.events?.append('deployment.started', { version: deployment.version, environment: deployment.environment }, { tenant: deployment.tenant, actor: deployment.started_by, subject: deployment.id });
    return deployment;
  }

  get(id) {
    return this.deployments.get(String(id));
  }

  require(id) {
    const deployment = this.get(id);
    if (!deployment) throw badRequest('deployment_missing', `deployment ${id} does not exist`);
    return deployment;
  }

  advance(id, options = {}) {
    const deployment = this.require(id);
    if (isFinal(deployment.status)) throw conflict('deployment_finished', `deployment ${id} is ${deployment.status}`);
    const following = nextStage(deployment.plan, stageOf(deployment));
    if (!following) {
      return this.finish(id, 'succeeded', options);
    }
    const next = {
      ...deployment,
      stage: following.name,
      stage_index: following.index,
      status: following.name === 'approve' ? 'awaiting_approval' : 'in_progress',
      updated_at: nowIso(this.clock),
    };
    this.deployments.set(id, next);
    return next;
  }

  approve(id, approval) {
    const deployment = this.require(id);
    const next = { ...deployment, approvals: [...deployment.approvals, approval], updated_at: nowIso(this.clock) };
    const plan = next.plan.find((stage) => stage.name === 'approve');
    if (plan && next.approvals.length >= plan.approvals && stageOf(next) === 'approve') {
      next.status = 'in_progress';
    }
    this.deployments.set(id, next);
    this.events?.append('approval.granted', { by: approval.by }, { tenant: next.tenant, actor: approval.by, subject: next.id });
    return next;
  }

  finish(id, status, options = {}) {
    const deployment = this.require(id);
    if (!['succeeded', 'failed'].includes(status)) throw badRequest('invalid_status', `${status} is not a final status`);
    const next = {
      ...deployment,
      status,
      stage: status === 'succeeded' ? 'done' : stageOf(deployment),
      finished_at: nowIso(this.clock),
      updated_at: nowIso(this.clock),
      failure: status === 'failed' ? String(options.reason ?? 'deployment failed') : null,
    };
    this.deployments.set(id, next);
    this.events?.append('deployment.finished', { status }, { tenant: next.tenant, actor: options.actor, subject: next.id });
    return next;
  }

  rollback(id, options = {}) {
    const deployment = this.require(id);
    if (!['in_progress', 'succeeded', 'failed'].includes(deployment.status)) {
      throw conflict('invalid_rollback', `deployment ${id} cannot be rolled back from ${deployment.status}`);
    }
    const next = {
      ...deployment,
      status: 'rolled_back',
      finished_at: nowIso(this.clock),
      updated_at: nowIso(this.clock),
      failure: String(options.reason ?? 'rolled back'),
    };
    this.deployments.set(id, next);
    this.events?.append('deployment.rolled_back', { reason: next.failure }, { tenant: next.tenant, actor: options.actor, subject: next.id });
    return next;
  }

  list(options = {}) {
    return [...this.deployments.values()]
      .filter((deployment) => options.tenant === undefined || deployment.tenant === options.tenant)
      .filter((deployment) => options.service === undefined || deployment.service === options.service)
      .filter((deployment) => options.environment === undefined || deployment.environment === options.environment)
      .filter((deployment) => options.status === undefined || deployment.status === options.status)
      .sort((left, right) => (left.started_at < right.started_at ? 1 : -1));
  }

  activeFor(service, environment) {
    return this.list({ service, environment }).find((deployment) => !isFinal(deployment.status));
  }

  open() {
    return this.list().filter((deployment) => !isFinal(deployment.status));
  }

  summary() {
    const byStatus = {};
    const byEnvironment = {};
    for (const deployment of this.deployments.values()) {
      byStatus[deployment.status] = (byStatus[deployment.status] ?? 0) + 1;
      byEnvironment[deployment.environment] = (byEnvironment[deployment.environment] ?? 0) + 1;
    }
    return { deployments: this.deployments.size, byStatus, byEnvironment, open: this.open().length };
  }

  stats() {
    return this.summary();
  }
}

export const STAGE_BUDGET_FACTOR = 1.5;

export function stageOverruns(deployment) {
  const plan = deployment.plan ?? [];
  const budget = plan.slice(0, deployment.stage_index + 1).reduce((total, stage) => total + stage.timeout_minutes, 0);
  const used = elapsedMinutes(deployment);
  return { budget_minutes: budget, used_minutes: used, over: used > budget * STAGE_BUDGET_FACTOR };
}

export function awaitingApproval(deployments) {
  return (deployments ?? []).filter((deployment) => deployment.status === 'awaiting_approval');
}

export function byEnvironment(deployments) {
  const out = new Map();
  for (const deployment of deployments ?? []) {
    const bucket = out.get(deployment.environment) ?? [];
    bucket.push(deployment);
    out.set(deployment.environment, bucket);
  }
  return out;
}

export function successRate(deployments) {
  const finished = (deployments ?? []).filter((deployment) => isFinal(deployment.status));
  const succeeded = finished.filter((deployment) => deployment.status === 'succeeded').length;
  return { finished: finished.length, succeeded, rate: finished.length === 0 ? 0 : Math.round((succeeded / finished.length) * 10000) / 100 };
}

export function slowestDeployment(deployments) {
  const finished = (deployments ?? []).filter((deployment) => deployment.finished_at);
  if (finished.length === 0) return undefined;
  return finished.reduce((slowest, deployment) =>
    elapsedMinutes(deployment) > elapsedMinutes(slowest) ? deployment : slowest,
  );
}

export function deploymentTimeline(deployment) {
  return (deployment.plan ?? []).map((stage) => ({
    stage: stage.name,
    position: stage.index,
    state: stage.index < deployment.stage_index ? 'done' : stage.index === deployment.stage_index ? 'current' : 'pending',
    manual: stage.manual,
  }));
}

export function rollbacksOf(deployments) {
  return (deployments ?? []).filter((deployment) => deployment.status === 'rolled_back');
}

export function deploymentsFor(releases, service) {
  return (releases ?? []).filter((release) => release.service === service).map((release) => release.id);
}

export function openInEnvironment(deployments, environment) {
  return (deployments ?? []).filter((deployment) => deployment.environment === environment && !isFinal(deployment.status));
}

export function meanElapsed(deployments, clock = () => new Date()) {
  const rows = (deployments ?? []).map((deployment) => elapsedMinutes(deployment, clock)).filter((value) => value >= 0);
  if (rows.length === 0) return 0;
  return Math.round((rows.reduce((total, value) => total + value, 0) / rows.length) * 100) / 100;
}
