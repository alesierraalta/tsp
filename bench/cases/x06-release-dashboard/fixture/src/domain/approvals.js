import { badRequest, conflict } from '../util/errors.js';
import { newId } from '../util/ids.js';
import { nowIso } from '../util/time.js';
import { can, roleOf } from '../auth/roles.js';

export const APPROVAL_STATES = ['pending', 'granted', 'refused'];

export function normalizeApproval(input = {}, options = {}) {
  const by = String(input.by ?? '').trim();
  if (by === '') throw badRequest('invalid_approval', 'an approval names who grants it');
  return {
    id: input.id ?? newId('apr'),
    deployment: String(input.deployment ?? options.deployment ?? ''),
    by,
    role: roleOf(input.role ?? 'approver'),
    state: 'pending',
    note: String(input.note ?? ''),
    requested_at: nowIso(options.clock),
    decided_at: null,
  };
}

export class ApprovalBook {
  constructor(options = {}) {
    this.clock = options.clock ?? (() => new Date());
    this.approvals = new Map();
  }

  request(deployment, input = {}) {
    if (!deployment?.id) throw badRequest('invalid_approval', 'an approval needs a deployment');
    const approval = normalizeApproval({ ...input, deployment: deployment.id }, { clock: this.clock });
    if (!can(approval.role, 'approve')) throw badRequest('invalid_approval', `${approval.role} may not approve`);
    if (this.pending(deployment.id).length > 0) throw conflict('approval_pending', `deployment ${deployment.id} already waits for an approval`);
    this.approvals.set(approval.id, approval);
    return approval;
  }

  get(id) {
    return this.approvals.get(String(id));
  }

  require(id) {
    const approval = this.get(id);
    if (!approval) throw badRequest('approval_missing', `approval ${id} does not exist`);
    return approval;
  }

  grant(id, options = {}) {
    const approval = this.require(id);
    if (approval.state !== 'pending') throw conflict('approval_decided', `approval ${id} is ${approval.state}`);
    const next = { ...approval, state: 'granted', decided_at: nowIso(this.clock), note: String(options.note ?? approval.note) };
    this.approvals.set(id, next);
    return next;
  }

  refuse(id, options = {}) {
    const approval = this.require(id);
    if (approval.state !== 'pending') throw conflict('approval_decided', `approval ${id} is ${approval.state}`);
    const next = { ...approval, state: 'refused', decided_at: nowIso(this.clock), note: String(options.reason ?? 'refused') };
    this.approvals.set(id, next);
    return next;
  }

  pending(deploymentId) {
    return [...this.approvals.values()].filter(
      (approval) => approval.state === 'pending' && (deploymentId === undefined || approval.deployment === deploymentId),
    );
  }

  list(deploymentId) {
    return [...this.approvals.values()]
      .filter((approval) => deploymentId === undefined || approval.deployment === deploymentId)
      .sort((left, right) => (left.requested_at < right.requested_at ? -1 : 1));
  }

  grantedFor(deploymentId) {
    return this.list(deploymentId).filter((approval) => approval.state === 'granted');
  }

  stats() {
    const byState = {};
    for (const approval of this.approvals.values()) byState[approval.state] = (byState[approval.state] ?? 0) + 1;
    return { approvals: this.approvals.size, byState };
  }
}

export function approvalSummary(approvals) {
  const granted = approvals.filter((approval) => approval.state === 'granted').length;
  const refused = approvals.filter((approval) => approval.state === 'refused').length;
  return { approvals: approvals.length, granted, refused, pending: approvals.length - granted - refused };
}

export function quorum(approvals, required) {
  const granted = (approvals ?? []).filter((approval) => approval.state === 'granted').length;
  const needed = Math.max(0, Math.trunc(required ?? 0));
  return { required: needed, granted, remaining: Math.max(0, needed - granted), met: granted >= needed };
}

export function pendingByDeployment(approvals) {
  const out = new Map();
  for (const approval of approvals ?? []) {
    if (approval.state !== 'pending') continue;
    const bucket = out.get(approval.deployment) ?? [];
    bucket.push(approval);
    out.set(approval.deployment, bucket);
  }
  return out;
}

export function slowestApproval(approvals) {
  const decided = (approvals ?? []).filter((approval) => approval.decided_at);
  if (decided.length === 0) return undefined;
  return decided.reduce((slowest, approval) =>
    new Date(approval.decided_at).getTime() - new Date(approval.requested_at).getTime() >
    new Date(slowest.decided_at).getTime() - new Date(slowest.requested_at).getTime()
      ? approval
      : slowest,
  );
}

export function approvalWaitMinutes(approval) {
  if (!approval?.decided_at) return undefined;
  return Math.round((new Date(approval.decided_at).getTime() - new Date(approval.requested_at).getTime()) / 60000);
}

export function approversNeeded(deployment, approvals) {
  const plan = deployment?.plan?.find((stage) => stage.name === 'approve');
  return quorum(approvals, plan?.approvals ?? 0);
}
