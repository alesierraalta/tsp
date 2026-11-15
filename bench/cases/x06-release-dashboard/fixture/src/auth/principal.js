import { unauthorized, forbidden } from '../util/errors.js';
import { apiKeyOf } from '../http/headers.js';

export const READ_SCOPES = ['releases:read', 'releases:write', 'admin'];
export const WRITE_SCOPES = ['releases:write', 'admin'];
export const WEBHOOK_SCOPES = ['webhooks:write', 'admin'];
export const ADMIN_SCOPES = ['admin'];

export function principalFromHeaders(headers, registry, options = {}) {
  const secret = apiKeyOf(headers);
  if (secret === '') return undefined;
  const record = registry.resolve(secret);
  return { key: record.id, id: record.id, tenant: record.tenant, scopes: record.scopes, label: record.label, via: options.via ?? 'api-key' };
}

export function requirePrincipal(ctx) {
  if (!ctx.principal) throw unauthorized('an api key is required');
  return ctx.principal;
}

export function hasScope(principal, scopes) {
  return (principal?.scopes ?? []).some((scope) => scopes.includes(scope));
}

export function requireRead(ctx) {
  const principal = requirePrincipal(ctx);
  if (!hasScope(principal, READ_SCOPES)) throw forbidden('this key may not read releases');
  return principal;
}

export function requireWrite(ctx) {
  const principal = requirePrincipal(ctx);
  if (!hasScope(principal, WRITE_SCOPES)) throw forbidden('this key may not change releases');
  return principal;
}

export function requireWebhooks(ctx) {
  const principal = requirePrincipal(ctx);
  if (!hasScope(principal, WEBHOOK_SCOPES)) throw forbidden('this key may not manage webhooks');
  return principal;
}

export function requireAdmin(ctx) {
  const principal = requirePrincipal(ctx);
  if (!hasScope(principal, ADMIN_SCOPES)) throw forbidden('this key needs the admin scope');
  return principal;
}

export function tenantOf(ctx) {
  return requirePrincipal(ctx).tenant;
}

export function scopeReport(principal) {
  return { scopes: [...(principal?.scopes ?? [])], tenant: principal?.tenant ?? null };
}
