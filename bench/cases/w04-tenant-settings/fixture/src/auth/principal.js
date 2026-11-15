import { forbidden, unauthorized } from '../http/errors.js';
import { bearerToken } from '../http/headers.js';
import { verifyToken } from './tokens.js';

export const RIGHTS = {
  owner: ['read', 'write', 'admin', 'billing'],
  admin: ['read', 'write', 'admin'],
  member: ['read', 'write'],
  viewer: ['read'],
  platform: ['read', 'write', 'admin', 'billing', 'platform'],
};

export function rightsOf(principal) {
  const rights = new Set();
  for (const role of principal?.roles ?? []) {
    for (const right of RIGHTS[role] ?? []) rights.add(right);
  }
  return rights;
}

export function can(principal, right, tenantId) {
  if (!principal) return false;
  if (principal.roles?.includes('platform')) return true;
  if (tenantId !== undefined && principal.tenantId !== tenantId) return false;
  return rightsOf(principal).has(right);
}

export function assertCan(principal, right, tenantId) {
  if (!principal) throw unauthorized();
  if (!can(principal, right, tenantId)) {
    throw forbidden(`the ${right} right is required for this tenant`);
  }
  return principal;
}

export function assertPlatform(principal) {
  if (!principal) throw unauthorized();
  if (!principal.roles?.includes('platform')) {
    throw forbidden('platform administration is required');
  }
  return principal;
}

export function principalFromToken(token, secret, options = {}) {
  const claims = verifyToken(token, secret, options);
  return normalizePrincipal(claims);
}

export function principalFromHeaders(headers, secret, options = {}) {
  const token = bearerToken(headers);
  if (!token) throw unauthorized();
  return principalFromToken(token, secret, options);
}

export function normalizePrincipal(claims) {
  const roles = Array.isArray(claims.roles) ? claims.roles.filter((role) => role in RIGHTS) : [];
  return {
    id: String(claims.sub ?? ''),
    tenantId: String(claims.tenant ?? ''),
    roles,
    email: claims.email === undefined ? undefined : String(claims.email),
  };
}

export function isSelf(principal, userId) {
  return Boolean(principal) && principal.id === userId;
}

export function describePrincipal(principal) {
  if (!principal) return 'anonymous';
  const roles = principal.roles?.length ? principal.roles.join('+') : 'none';
  return `${principal.id || 'unknown'} (${roles})`;
}

export function assertSameTenant(principal, tenantId) {
  if (!principal) throw unauthorized();
  if (principal.roles?.includes('platform')) return principal;
  if (principal.tenantId !== tenantId) throw forbidden('this tenant is not yours');
  return principal;
}
