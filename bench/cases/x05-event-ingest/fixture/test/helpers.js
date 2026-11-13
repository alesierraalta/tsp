import { createApp, startApp } from '../src/server.js';

export const FIXED_NOW = new Date('2026-02-10T12:00:00.000Z');

export const clock = () => FIXED_NOW;

export function appWithKeys(options = {}) {
  const app = createApp({ clock, ...options });
  const issued = {
    admin: app.keys.issue('acme', { scopes: ['admin'], label: 'admin' }),
    writer: app.keys.issue('acme', { scopes: ['ingest:write'], label: 'writer' }),
    reader: app.keys.issue('acme', { scopes: ['reports:read'], label: 'reader' }),
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
    admin: started.app.keys.issue('acme', { scopes: ['admin'], label: 'admin' }),
    writer: started.app.keys.issue('acme', { scopes: ['ingest:write'], label: 'writer' }),
    reader: started.app.keys.issue('acme', { scopes: ['reports:read'], label: 'reader' }),
    other: started.app.keys.issue('globex', { scopes: ['admin'], label: 'globex admin' }),
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

export function pageView(overrides = {}) {
  return {
    name: 'page_view',
    timestamp: '2026-02-09T10:00:00.000Z',
    properties: { path: overrides.path ?? '/pricing' },
    context: { user: overrides.user ?? 'usr_1', path: overrides.path ?? '/pricing', session: overrides.session ?? 'ses_1' },
    ...(overrides.body ?? {}),
  };
}

export function seedEvents(app, tenant = 'acme', count = 3) {
  const accepted = [];
  for (let index = 0; index < count; index += 1) {
    const result = app.intake.accept(tenant, [
      pageView({
        user: `usr_${index + 1}`,
        path: index === 0 ? '/pricing' : '/docs',
        body: { id: `evt_fixed_${index + 1}`, timestamp: `2026-02-0${index + 8}T10:00:00.000Z` },
      }),
    ]);
    accepted.push(result);
  }
  return accepted;
}
