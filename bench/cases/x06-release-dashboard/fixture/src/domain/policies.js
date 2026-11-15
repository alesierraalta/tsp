import { badRequest } from '../util/errors.js';
import { approvalsRequired, environmentOf, isProtected } from './environments.js';
import { roleRank, isRole } from '../auth/roles.js';

// A policy answers one question per deployment: may this actor deploy this release to this environment. The
// answer is derived from the environment's tier, the actor's role and the release's channel, so a policy is
// data a route can report instead of logic the route has to repeat.
export const POLICY_KINDS = ['deploy', 'approve', 'rollback', 'promote'];

export function normalizePolicy(input = {}) {
  const kind = POLICY_KINDS.includes(input.kind) ? input.kind : 'deploy';
  const environment = environmentOf(input.environment);
  const minimumRole = isRole(input.minimum_role) ? input.minimum_role : 'releaser';
  return {
    id: String(input.id ?? `${kind}:${environment}`),
    kind,
    environment,
    minimum_role: minimumRole,
    approvals: input.approvals === undefined ? approvalsRequired(environment) : Math.max(0, Math.trunc(Number(input.approvals))),
    allowed_channels: Array.isArray(input.allowed_channels) ? input.allowed_channels.map(String) : [],
    require_freeze_override: input.require_freeze_override === true,
  };
}

export function evaluate(policy, context = {}) {
  const reasons = [];
  const role = isRole(context.role) ? context.role : 'viewer';
  if (roleRank(role) < roleRank(policy.minimum_role)) reasons.push(`role ${role} is below ${policy.minimum_role}`);
  if ((policy.allowed_channels ?? []).length > 0 && context.channel !== undefined && !policy.allowed_channels.includes(context.channel)) {
    reasons.push(`channel ${context.channel} is not admitted`);
  }
  if (policy.require_freeze_override && context.freeze === true && context.override !== true) {
    reasons.push('a freeze is active and the release carries no override');
  }
  if (context.approvals !== undefined && policy.approvals > 0 && Number(context.approvals) < policy.approvals) {
    reasons.push(`${policy.approvals} approval(s) required, ${context.approvals} recorded`);
  }
  return { allowed: reasons.length === 0, reasons, policy: policy.id };
}

export class PolicyBook {
  constructor(options = {}) {
    this.policies = new Map();
    const defaults = options.defaults ?? [
      { kind: 'deploy', environment: 'development', minimum_role: 'releaser', approvals: 0 },
      { kind: 'deploy', environment: 'staging', minimum_role: 'releaser', approvals: 1 },
      { kind: 'deploy', environment: 'production', minimum_role: 'approver', approvals: 2 },
      { kind: 'rollback', environment: 'production', minimum_role: 'releaser', approvals: 0 },
    ];
    for (const entry of defaults) {
      const policy = normalizePolicy(entry);
      this.policies.set(policy.id, policy);
    }
  }

  define(input) {
    const policy = normalizePolicy(input);
    this.policies.set(policy.id, policy);
    return policy;
  }

  get(id) {
    return this.policies.get(String(id));
  }

  for(kind, environment) {
    return this.get(`${kind}:${environmentOf(environment)}`);
  }

  list() {
    return [...this.policies.values()].sort((left, right) => (left.id < right.id ? -1 : 1));
  }

  check(kind, environment, context = {}) {
    const policy = this.for(kind, environment);
    if (!policy) return { allowed: false, reasons: [`no policy for ${kind} in ${environment}`], policy: null };
    return evaluate(policy, context);
  }

  matrix(environments) {
    return (environments ?? []).map((environment) => ({
      environment,
      protected: isProtected(environment),
      deploy: this.check('deploy', environment, { role: 'releaser' }).reasons,
      approve: this.check('approve', environment, { role: 'approver' }).reasons,
    }));
  }

  stats() {
    const byKind = {};
    for (const policy of this.policies.values()) byKind[policy.kind] = (byKind[policy.kind] ?? 0) + 1;
    return { policies: this.policies.size, byKind };
  }
}

export function describePolicy(policy) {
  return {
    id: policy.id,
    kind: policy.kind,
    environment: policy.environment,
    minimum_role: policy.minimum_role,
    approvals: policy.approvals,
    allowed_channels: [...policy.allowed_channels],
  };
}

export function policyRows(book) {
  return book.list().map(describePolicy);
}

export function assertPolicy(book, kind, environment, context) {
  const decision = book.check(kind, environment, context);
  if (!decision.allowed) throw badRequest('policy_refused', decision.reasons[0] ?? 'policy refused', { reasons: decision.reasons });
  return decision;
}
