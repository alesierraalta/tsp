import { rolloutStepsOf, environmentOf, isProtected } from './environments.js';
import { badRequest } from '../util/errors.js';

export const OBSERVATION_MINUTES = { development: 1, staging: 10, production: 30 };

export function rolloutPlan(environment, options = {}) {
  const name = environmentOf(environment);
  const steps = [...(options.steps ?? rolloutStepsOf(name))];
  if (steps.length === 0) throw badRequest('invalid_rollout', 'a rollout needs at least one step');
  const observe = options.observeMinutes ?? OBSERVATION_MINUTES[name] ?? 10;
  return {
    environment: name,
    protected: isProtected(name),
    steps: steps.map((percent, index) => ({
      index,
      percent: Math.max(1, Math.min(100, Math.trunc(percent))),
      observe_minutes: observe,
      checks: index === steps.length - 1 ? ['error_rate', 'latency', 'health'] : ['error_rate', 'health'],
    })),
  };
}

export function percentAt(plan, index) {
  return plan.steps[Math.max(0, Math.min(plan.steps.length - 1, Math.trunc(index)))].percent;
}

export function advanceStep(plan, index) {
  const next = Math.trunc(index) + 1;
  return next >= plan.steps.length ? undefined : plan.steps[next];
}

export function isComplete(plan, index) {
  return Math.trunc(index) >= plan.steps.length - 1;
}

export function rolloutMinutes(plan) {
  return plan.steps.reduce((total, step) => total + step.observe_minutes, 0);
}

export function describeRollout(plan, index = 0) {
  const step = plan.steps[Math.max(0, Math.min(plan.steps.length - 1, Math.trunc(index)))];
  return {
    environment: plan.environment,
    steps: plan.steps.length,
    current: step.percent,
    next: advanceStep(plan, index)?.percent,
    complete: isComplete(plan, index),
    minutes: rolloutMinutes(plan),
  };
}

export function rolloutTable(environments) {
  return environments.map((name) => {
    const plan = rolloutPlan(name);
    return { environment: name, steps: plan.steps.map((step) => step.percent), minutes: rolloutMinutes(plan) };
  });
}

export function splitPercent(percent) {
  const whole = Math.round((Number(percent) || 0) * 100) / 100;
  return { percent: whole, asFraction: whole / 100, label: `${whole}%` };
}
