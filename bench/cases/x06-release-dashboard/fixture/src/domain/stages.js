import { approvalsRequired, rolloutStepsOf, environmentOf, isProtected } from './environments.js';
import { diffMilliseconds } from '../util/time.js';

export const STAGES = ['prepare', 'approve', 'rollout', 'verify', 'done'];

export const STAGE_LABELS = {
  prepare: 'Prepare the artifacts',
  approve: 'Collect the approvals',
  rollout: 'Roll the version out',
  verify: 'Verify the error budget',
  done: 'Close the deployment',
};

export const STAGE_TIMEOUTS = {
  prepare: 30,
  approve: 120,
  rollout: 240,
  verify: 60,
  done: 5,
};

export function stagePlan(environment, options = {}) {
  const name = environmentOf(environment);
  const approvals = options.approvals ?? approvalsRequired(name);
  const rollout = options.rollout ?? rolloutStepsOf(name);
  return STAGES.map((stage, index) => ({
    name: stage,
    index,
    label: STAGE_LABELS[stage],
    manual: stage === 'approve' || (stage === 'rollout' && isProtected(name)),
    approvals: stage === 'approve' ? approvals : 0,
    rollout: stage === 'rollout' ? [...rollout] : [],
    timeout_minutes: options.timeouts?.[stage] ?? STAGE_TIMEOUTS[stage],
  }));
}

export function nextStage(plan, current) {
  const index = plan.findIndex((entry) => entry.name === current);
  if (index < 0) return undefined;
  return plan[index + 1];
}

export function stageAt(plan, index) {
  return plan[Math.max(0, Math.min(plan.length - 1, Math.trunc(index)))];
}

export function stageStatusOf(plan, index, currentIndex) {
  const position = plan.findIndex((entry) => entry.index === index);
  if (position < 0) return 'unknown';
  if (position < currentIndex) return 'done';
  if (position === currentIndex) return 'current';
  return 'pending';
}

export function stageDuration(stage) {
  if (!stage?.started_at) return 0;
  return diffMilliseconds(stage.started_at, stage.finished_at ?? new Date());
}

export function describePlan(plan) {
  return {
    stages: plan.length,
    manual: plan.filter((stage) => stage.manual).length,
    approvals: plan.reduce((total, stage) => total + stage.approvals, 0),
    rollout: plan.find((stage) => stage.name === 'rollout')?.rollout ?? [],
    minutes: plan.reduce((total, stage) => total + stage.timeout_minutes, 0),
  };
}

export function planTimeline(plan) {
  let minutes = 0;
  return plan.map((stage) => {
    const start = minutes;
    minutes += stage.timeout_minutes;
    return { stage: stage.name, from_minutes: start, to_minutes: minutes };
  });
}

export function isFinal(stage) {
  return stage === 'done';
}

export function gatesOf(plan) {
  return plan
    .filter((stage) => stage.manual || stage.approvals > 0)
    .map((stage) => ({ stage: stage.name, approvals: stage.approvals, manual: stage.manual }));
}

export function stageIndex(plan, name) {
  return plan.findIndex((stage) => stage.name === name);
}

export function remainingMinutes(plan, currentIndex) {
  return plan.slice(currentIndex).reduce((total, stage) => total + stage.timeout_minutes, 0);
}

export function isManual(plan, name) {
  return plan.find((stage) => stage.name === name)?.manual === true;
}

export function stageLabel(name) {
  return STAGE_LABELS[name] ?? name;
}

export function advance(plan, currentIndex) {
  const next = Math.trunc(currentIndex) + 1;
  return next >= plan.length ? undefined : plan[next];
}

export function planReport(plan, currentIndex = 0) {
  return {
    stages: plan.length,
    current: plan[currentIndex]?.name,
    remaining_minutes: remainingMinutes(plan, currentIndex),
    gates: gatesOf(plan).length,
    done: currentIndex >= plan.length - 1,
  };
}
