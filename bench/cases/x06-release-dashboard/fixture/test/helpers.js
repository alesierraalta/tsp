import { createApp, startApp } from '../src/server.js';

export const FIXED_NOW = new Date('2026-02-10T12:00:00.000Z');
export const clock = () => FIXED_NOW;

export const TENANT = 'acme';

export function appWithKeys(options = {}) {
  const app = createApp({ clock, ...options });
  const issued = {
    admin: app.keys.issue(TENANT, { scopes: ['admin'], label: 'admin' }),
    releaser: app.keys.issue(TENANT, { scopes: ['releases:write'], label: 'releaser' }),
    reader: app.keys.issue(TENANT, { scopes: ['releases:read'], label: 'reader' }),
    hooks: app.keys.issue(TENANT, { scopes: ['webhooks:write'], label: 'hooks' }),
    other: app.keys.issue('globex', { scopes: ['admin'], label: 'globex admin' }),
  };
  return { app, issued };
}

export function apiKey(secret) {
  return { 'x-api-key': secret, 'content-type': 'application/json' };
}

export async function withApp(options = {}) {
  const started = await startApp({ clock, ...options });
  const issued = {
    admin: started.app.keys.issue(TENANT, { scopes: ['admin'], label: 'admin' }),
    releaser: started.app.keys.issue(TENANT, { scopes: ['releases:write'], label: 'releaser' }),
    reader: started.app.keys.issue(TENANT, { scopes: ['releases:read'], label: 'reader' }),
    hooks: started.app.keys.issue(TENANT, { scopes: ['webhooks:write'], label: 'hooks' }),
  };
  return {
    ...started,
    issued,
    async stop() {
      await started.close();
    },
  };
}

export async function call(url, path, options = {}) {
  const response = await fetch(`${url}${path}`, { ...options, signal: AbortSignal.timeout(5000) });
  const text = await response.text();
  let body;
  try {
    body = JSON.parse(text);
  } catch {
    body = text;
  }
  return { status: response.status, body, headers: response.headers };
}

export function releaseInput(overrides = {}) {
  return {
    version: overrides.version ?? '1.4.0',
    service: overrides.service ?? 'checkout',
    environment: overrides.environment ?? 'development',
    summary: overrides.summary ?? 'Checkout release',
    changelog: overrides.changelog ?? [
      { kind: 'feature', summary: 'Add a saved-card selector' },
      { kind: 'fix', summary: 'Stop double charging on retry' },
    ],
  };
}

export function seedRelease(app, overrides = {}) {
  return app.releases.create({ ...releaseInput(overrides), tenant: TENANT, created_by: 'usr_1' });
}

export function seedDeployment(app, overrides = {}) {
  const release = seedRelease(app, overrides);
  return { release, deployment: app.deployments.start(release, { actor: 'usr_1' }) };
}
