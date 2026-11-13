import { forbidden, unauthorized } from '../util/errors.js';
import { verifyToken } from './tokens.js';
import { bearerOf } from '../http/headers.js';

const READ_ROLES = ['viewer', 'editor', 'admin'];
const WRITE_ROLES = ['editor', 'admin'];
const ADMIN_ROLES = ['admin'];

export function principalFromHeaders(headers, secret, options = {}) {
  const token = bearerOf(headers);
  if (token === '') return undefined;
  const payload = verifyToken(token, secret, options);
  return {
    id: String(payload.sub ?? ''),
    tenant: String(payload.tenant ?? 'default'),
    roles: Array.isArray(payload.roles) ? payload.roles.map((role) => String(role).toLowerCase()) : [],
  };
}

export function requirePrincipal(ctx) {
  if (!ctx.principal) throw unauthorized('authentication required');
  return ctx.principal;
}

export function hasRole(principal, roles) {
  return (principal?.roles ?? []).some((role) => roles.includes(role));
}

export function requireRead(ctx) {
  const principal = requirePrincipal(ctx);
  if (!hasRole(principal, READ_ROLES)) throw forbidden('this token may not read content');
  return principal;
}

export function requireWrite(ctx) {
  const principal = requirePrincipal(ctx);
  if (!hasRole(principal, WRITE_ROLES)) throw forbidden('this token may not write content');
  return principal;
}

export function requireAdmin(ctx) {
  const principal = requirePrincipal(ctx);
  if (!hasRole(principal, ADMIN_ROLES)) throw forbidden('this token needs the admin role');
  return principal;
}

export function canWrite(principal) {
  return hasRole(principal, WRITE_ROLES);
}
