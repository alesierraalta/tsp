import { mkdtemp, rm, mkdir } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { createApp, startApp } from '../src/server.js';
import { signToken } from '../src/auth/tokens.js';

export const SECRET = 'test-secret-value';

export async function tempRoots() {
  const base = await mkdtemp(join(tmpdir(), 'hub-test-'));
  const roots = {
    base,
    media: join(base, 'media'),
    templates: join(base, 'templates'),
    plugins: join(base, 'plugins'),
    exports: join(base, 'exports'),
    data: join(base, 'data'),
  };
  for (const dir of Object.values(roots)) {
    if (dir !== base) await mkdir(dir, { recursive: true });
  }
  return roots;
}

export async function cleanup(roots) {
  await rm(roots.base, { recursive: true, force: true });
}

export function token(roles = ['admin'], extra = {}) {
  return signToken({ sub: 'usr_tester', tenant: 'acme', roles, ...extra }, SECRET);
}

export function authHeaders(roles = ['admin']) {
  return { authorization: `Bearer ${token(roles)}`, 'content-type': 'application/json' };
}

export async function withApp(options = {}) {
  const roots = await tempRoots();
  const started = await startApp({
    secret: SECRET,
    mediaRoot: roots.media,
    templateRoot: roots.templates,
    pluginRoot: roots.plugins,
    pluginAssetRoot: join(roots.plugins, 'assets'),
    exportRoot: roots.exports,
    ...options,
  });
  await mkdir(join(roots.plugins, 'assets'), { recursive: true });
  return {
    ...started,
    roots,
    async stop() {
      await started.close();
      await cleanup(roots);
    },
  };
}

export async function call(url, path, options = {}) {
  const response = await fetch(`${url}${path}`, options);
  const text = await response.text();
  let body;
  try {
    body = JSON.parse(text);
  } catch {
    body = text;
  }
  return { status: response.status, body, headers: response.headers };
}

export function articleInput(overrides = {}) {
  return {
    values: {
      title: overrides.title ?? 'Hello world',
      body: overrides.body ?? 'First paragraph of the article.',
      topic: overrides.topic ?? 'news',
      ...(overrides.values ?? {}),
    },
    locale: overrides.locale,
  };
}

export function appWithoutServer(options = {}) {
  return createApp({
    secret: SECRET,
    mediaRoot: options.mediaRoot,
    templateRoot: options.templateRoot,
    pluginRoot: options.pluginRoot,
    exportRoot: options.exportRoot,
    ...options,
  });
}
