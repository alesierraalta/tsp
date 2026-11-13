import { forbidden } from '../util/errors.js';

export const ROLES = ['viewer', 'releaser', 'approver', 'owner'];

export const ROLE_CAPABILITIES = {
  viewer: ['read'],
  releaser: ['read', 'deploy', 'rollback'],
  approver: ['read', 'approve', 'deploy'],
  owner: ['read', 'deploy', 'approve', 'rollback', 'manage'],
};

export function isRole(value) {
  return ROLES.includes(String(value ?? ''));
}

export function roleOf(value, fallback = 'viewer') {
  return isRole(value) ? String(value) : fallback;
}

export function roleRank(value) {
  return ROLES.indexOf(roleOf(value));
}

export function capabilitiesOf(role) {
  return [...(ROLE_CAPABILITIES[roleOf(role)] ?? ROLE_CAPABILITIES.viewer)];
}

export function can(role, capability) {
  return capabilitiesOf(role).includes(String(capability));
}

export function requireCapability(role, capability) {
  if (!can(role, capability)) throw forbidden(`${roleOf(role)} may not ${capability}`);
  return true;
}

export function isHigherRole(left, right) {
  return roleRank(left) > roleRank(right);
}

export function rolesAbove(role) {
  const rank = roleRank(role);
  return ROLES.filter((candidate, index) => index > rank);
}

export function describeRole(role) {
  return { role: roleOf(role), rank: roleRank(role), capabilities: capabilitiesOf(role) };
}

export function mergeRoles(roles) {
  const set = new Set((roles ?? []).filter(isRole));
  return [...set].sort((left, right) => roleRank(right) - roleRank(left));
}

export function strongestRole(roles) {
  return mergeRoles(roles)[0] ?? 'viewer';
}

export function assignRole(members, user, role) {
  const next = new Map(members instanceof Map ? members : Object.entries(members ?? {}));
  next.set(String(user), roleOf(role));
  return next;
}

export function membersWith(members, capability) {
  const entries = members instanceof Map ? [...members.entries()] : Object.entries(members ?? {});
  return entries.filter(([, role]) => can(role, capability)).map(([user]) => user).sort();
}
