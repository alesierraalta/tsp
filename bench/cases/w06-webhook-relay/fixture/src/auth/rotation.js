export const DEFAULT_ROTATION = {
  maxAgeDays: 30,
  graceHours: 6,
  minKeys: 1,
};

export const DAY_MS = 24 * 60 * 60 * 1000;

export function ageOf(key, now = new Date()) {
  if (!key || key.created_at === undefined) throw new TypeError('a key needs a created_at date');
  const created = new Date(key.created_at).getTime();
  const at = new Date(now).getTime();
  if (Number.isNaN(created) || Number.isNaN(at)) throw new TypeError('a key needs a created_at date');
  return Math.max(0, at - created);
}

export function ageDays(key, now = new Date()) {
  return Math.floor(ageOf(key, now) / DAY_MS);
}

export function rotationDue(key, policy = DEFAULT_ROTATION, now = new Date()) {
  return ageDays(key, now) >= policy.maxAgeDays;
}

export function withinGrace(key, policy = DEFAULT_ROTATION, now = new Date()) {
  const limit = (policy.maxAgeDays + policy.graceHours / 24) * DAY_MS;
  return ageOf(key, now) <= limit;
}

export function planRotation(keys, policy = DEFAULT_ROTATION, now = new Date()) {
  const active = keys.filter((key) => key.active !== false);
  const due = active.filter((key) => rotationDue(key, policy, now));
  const expired = keys.filter((key) => !withinGrace(key, policy, now));
  return {
    rotate: due.length > 0,
    due: due.map((key) => key.id),
    expired: expired.map((key) => key.id),
    reason: due.length > 0 ? 'age' : 'fresh',
    active: active.length,
  };
}

export function usableKeys(keys, policy = DEFAULT_ROTATION, now = new Date()) {
  return keys.filter((key) => key.active !== false && withinGrace(key, policy, now));
}

export function rotationSummary(keys, policy = DEFAULT_ROTATION, now = new Date()) {
  const plan = planRotation(keys, policy, now);
  return `active=${plan.active} due=${plan.due.length} expired=${plan.expired.length} reason=${plan.reason}`;
}
