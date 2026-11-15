import test from 'node:test';
import assert from 'node:assert/strict';
import { STAGES, stagePlan, nextStage, stageStatusOf, describePlan, gatesOf, planReport, remainingMinutes } from '../src/domain/stages.js';
import { DeploymentBook, normalizeDeployment, stageOf, progressOf, isFinal, successRate, awaitingApproval, stageOverruns } from '../src/domain/deployments.js';
import { ApprovalBook, normalizeApproval, quorum, approvalSummary, approversNeeded } from '../src/domain/approvals.js';
import { rolloutPlan, percentAt, advanceStep, isComplete, rolloutMinutes, describeRollout, rolloutTable } from '../src/domain/rollouts.js';
import { IncidentBook, normalizeIncident, severityOf, severityRank, isBlocking, openBySeverity, resolutionRate } from '../src/domain/incidents.js';
import { ReleaseBook } from '../src/domain/releases.js';
import { clock, releaseInput } from './helpers.js';

function fixture(overrides = {}) {
  const releases = new ReleaseBook({ clock });
  const release = releases.create({ ...releaseInput(overrides), tenant: 'acme', created_by: 'usr_1' });
  const deployments = new DeploymentBook({ clock });
  const approvals = new ApprovalBook({ clock });
  const incidents = new IncidentBook({ clock });
  return { releases, release, deployments, approvals, incidents };
}

test('a stage plan carries the approvals and rollout steps of its environment', () => {
  const plan = stagePlan('production');
  assert.deepEqual(STAGES, ['prepare', 'approve', 'rollout', 'verify', 'done']);
  assert.equal(plan.length, 5);
  assert.equal(plan.find((stage) => stage.name === 'approve').approvals, 2);
  assert.deepEqual(plan.find((stage) => stage.name === 'rollout').rollout, [1, 10, 50, 100]);
  assert.equal(nextStage(plan, 'prepare').name, 'approve');
  assert.equal(nextStage(plan, 'done'), undefined);
  assert.equal(stageStatusOf(plan, 0, 2), 'done');
  assert.equal(stageStatusOf(plan, 2, 2), 'current');
  assert.equal(stageStatusOf(plan, 3, 2), 'pending');
  assert.equal(describePlan(plan).stages, 5);
  assert.equal(describePlan(plan).approvals, 2);
  assert.equal(describePlan(plan).manual >= 2, true);
  assert.equal(gatesOf(plan).length >= 2, true);
  assert.equal(remainingMinutes(plan, 0) > 0, true);
  assert.equal(planReport(plan, 0).current, 'prepare');
  assert.equal(stagePlan('development').find((stage) => stage.name === 'approve').approvals, 0);
});

test('a deployment walks its plan and refuses a second move once it finished', () => {
  const { release, deployments } = fixture();
  const deployment = deployments.start(release, { actor: 'usr_1' });
  assert.equal(deployment.status, 'pending');
  assert.equal(stageOf(deployment), 'prepare');
  assert.equal(progressOf(deployment), 20);
  assert.throws(() => deployments.start(release, { actor: 'usr_1' }), /already deploying/);
  const advanced = deployments.advance(deployment.id);
  assert.equal(advanced.stage, 'approve');
  assert.equal(advanced.status, 'awaiting_approval');
  const finished = deployments.finish(deployment.id, 'succeeded');
  assert.equal(finished.status, 'succeeded');
  assert.equal(isFinal(finished.status), true);
  assert.throws(() => deployments.advance(deployment.id), /is succeeded/);
  assert.equal(awaitingApproval(deployments.list()).length, 0);
});

test('a production deployment needs the approvals its plan asks for', () => {
  const { release, deployments, approvals } = fixture({ environment: 'production' });
  const deployment = deployments.start(release, { actor: 'usr_1' });
  const requested = approvals.request(deployment, { by: 'usr_approver', role: 'approver' });
  assert.equal(requested.state, 'pending');
  assert.equal(approvals.pending(deployment.id).length, 1);
  assert.throws(() => approvals.request(deployment, { by: 'usr_2', role: 'approver' }), /already waits/);
  const granted = approvals.grant(requested.id, { note: 'looks fine' });
  assert.equal(granted.state, 'granted');
  assert.equal(approvals.grantedFor(deployment.id).length, 1);
  assert.deepEqual(quorum(approvals.list(deployment.id), 2), { required: 2, granted: 1, remaining: 1, met: false });
  assert.equal(approversNeeded(deployment, approvals.list(deployment.id)).granted, 1);
  assert.equal(approvalSummary(approvals.list(deployment.id)).granted, 1);
  assert.equal(approvals.stats().byState.granted, 1);
  assert.throws(() => approvals.grant(requested.id), /is granted/);
  assert.throws(() => normalizeApproval({ by: '' }), /names who grants/);
});

test('a refused approval fails the deployment it was asked for', () => {
  const { release, deployments, approvals } = fixture({ environment: 'production' });
  const deployment = deployments.start(release, { actor: 'usr_1' });
  const requested = approvals.request(deployment, { by: 'usr_approver', role: 'approver' });
  const refused = approvals.refuse(requested.id, { reason: 'not yet' });
  assert.equal(refused.state, 'refused');
  const failed = deployments.finish(deployment.id, 'failed', { reason: `approval refused: ${refused.note}` });
  assert.equal(failed.status, 'failed');
  assert.match(failed.failure, /not yet/);
});

test('a rollout walks the percentages its environment declares', () => {
  const plan = rolloutPlan('production');
  assert.deepEqual(plan.steps.map((step) => step.percent), [1, 10, 50, 100]);
  assert.equal(percentAt(plan, 1), 10);
  assert.equal(advanceStep(plan, 0).percent, 10);
  assert.equal(isComplete(plan, 3), true);
  assert.equal(rolloutMinutes(plan), 120);
  assert.deepEqual(describeRollout(plan, 0), { environment: 'production', steps: 4, current: 1, next: 10, complete: false, minutes: 120 });
  assert.equal(rolloutTable(['staging'])[0].steps.length, 2);
});

test('incidents rank their severity and block a promotion above sev3', () => {
  const incident = normalizeIncident({ title: 'Checkout is down', severity: 'sev1', service: 'checkout' }, { clock });
  assert.equal(severityOf('nope'), 'sev3');
  assert.equal(severityRank('sev1') > severityRank('sev3'), true);
  assert.equal(isBlocking('sev2'), true);
  assert.equal(isBlocking('sev4'), false);
  const { incidents } = fixture();
  const opened = incidents.open({ title: 'Latency', severity: 'sev2', service: 'checkout' });
  incidents.open({ title: 'Typo', severity: 'sev4', service: 'search' });
  assert.equal(incidents.blocking().length, 1);
  assert.equal(openBySeverity(incidents.list()).sev2, 1);
  assert.equal(incidents.list({ service: 'search' }).length, 1);
  assert.equal(incidents.resolve(opened.id, { resolution: 'rolled back' }).state, 'resolved');
  assert.throws(() => incidents.resolve(opened.id), /is resolved/);
  assert.deepEqual(resolutionRate(incidents.list()), { total: 2, resolved: 1, rate: 50 });
  assert.equal(incidents.summary().open, 1);
  assert.equal(incident.title, 'Checkout is down');
});

test('a deployment reports its success rate and stage overruns', () => {
  const { release, deployments } = fixture();
  const first = deployments.start(release, { actor: 'usr_1' });
  deployments.finish(first.id, 'succeeded');
  const second = deployments.start({ ...release, id: 'rel_other' }, { actor: 'usr_1' });
  deployments.finish(second.id, 'failed', { reason: 'health check' });
  assert.deepEqual(successRate(deployments.list()), { finished: 2, succeeded: 1, rate: 50 });
  assert.equal(deployments.summary().byStatus.failed, 1);
  assert.equal(stageOverruns(deployments.get(first.id)).over, false);
  assert.equal(progressOf(deployments.get(first.id)) > 0, true);
  assert.equal(normalizeDeployment(release).plan.length, 5);
});
