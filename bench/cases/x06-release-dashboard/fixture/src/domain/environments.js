import { titleCase } from '../util/text.js';
import { badRequest } from '../util/errors.js';

export const ENVIRONMENTS = ['development', 'staging', 'production'];

export const ENV_TIERS = {
  development: { tier: 'low', protected: false, approvals: 0, rollout: [100] },
  staging: { tier: 'medium', protected: false, approvals: 1, rollout: [50, 100] },
  production: { tier: 'high', protected: true, approvals: 2, rollout: [1, 10, 50, 100] },
};

export function environmentOf(name, fallback = 'development') {
  const value = String(name ?? '').toLowerCase();
  return ENVIRONMENTS.includes(value) ? value : fallback;
}

export function normalizeEnvironment(input = {}) {
  const name = String(input.name ?? '').toLowerCase();
  if (!ENVIRONMENTS.includes(name)) throw badRequest('invalid_environment', `environment ${input.name} is not known`);
  const tier = ENV_TIERS[name];
  return {
    name,
    display: titleCase(name),
    tier: tier.tier,
    protected: input.protected === undefined ? tier.protected : input.protected === true,
    approvals: input.approvals === undefined ? tier.approvals : Number(input.approvals),
    rollout: [...(input.rollout ?? tier.rollout)],
  };
}

export function isProtected(name) {
  return ENV_TIERS[environmentOf(name)].protected;
}

export function approvalsRequired(name) {
  return ENV_TIERS[environmentOf(name)].approvals;
}

export function rolloutStepsOf(name) {
  return [...ENV_TIERS[environmentOf(name)].rollout];
}

export function describeEnvironment(name) {
  const environment = normalizeEnvironment({ name });
  return {
    name: environment.name,
    display: environment.display,
    tier: environment.tier,
    protected: environment.protected,
    approvals: environment.approvals,
    rollout: environment.rollout,
  };
}

export function environmentTable() {
  return ENVIRONMENTS.map((name) => describeEnvironment(name));
}

export function compareEnvironments(left, right) {
  return ENVIRONMENTS.indexOf(environmentOf(left)) - ENVIRONMENTS.indexOf(environmentOf(right));
}

export function higherOf(left, right) {
  return compareEnvironments(left, right) >= 0 ? environmentOf(left) : environmentOf(right);
}
