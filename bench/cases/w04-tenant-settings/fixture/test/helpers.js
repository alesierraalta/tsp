import { mkdtemp, rm } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { startApp } from '../src/server.js';
import { signToken } from '../src/auth/tokens.js';

export const SECRET = 'test-secret-for-tenant-gateway';

export async function tempDir(prefix = 'w04-') {
  return mkdtemp(join(tmpdir(), prefix));
}

export async function cleanup(dir) {
  await rm(dir, { recursive: true, force: true });
}

export function tokenFor(claims, ttlSeconds = 600) {
  return signToken({ exp: undefined, ...claims }, SECRET, { ttlSeconds });
}

export async function withApp(fn, options = {}) {
  const dir = await tempDir();
  const started = await startApp({ secret: SECRET, dataRoot: dir, ...options });
  try {
    return await fn({ ...started, dir });
  } finally {
    await started.close();
    await cleanup(dir);
  }
}

export async function call(url, method, path, options = {}) {
  const headers = { ...(options.headers ?? {}) };
  if (options.token) headers.authorization = `Bearer ${options.token}`;
  if ((options.body !== undefined || options.raw !== undefined) && headers['content-type'] === undefined) {
    headers['content-type'] = 'application/json';
  }
  const response = await fetch(`${url}${path}`, {
    method,
    headers,
    body: options.raw ?? (options.body === undefined ? undefined : JSON.stringify(options.body)),
  });
  const text = await response.text();
  let json;
  try {
    json = text.length === 0 ? undefined : JSON.parse(text);
  } catch {
    json = undefined;
  }
  return { status: response.status, headers: response.headers, text, json };
}

export function seedTenant(app, overrides = {}) {
  return app.tenants.create({ name: 'Acme', plan: 'team', settings: {}, ...overrides });
}

export function seedUser(app, tenantId, overrides = {}) {
  return app.users.create({
    tenant_id: tenantId,
    email: 'sam@example.com',
    name: 'Sam Member',
    roles: ['member'],
    settings: {},
    ...overrides,
  });
}
