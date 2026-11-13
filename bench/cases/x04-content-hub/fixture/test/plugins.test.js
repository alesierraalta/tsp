import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';
import { join } from 'node:path';
import { parseManifest, manifestSummary, compareVersions, requiresPermission } from '../src/plugins/manifest.js';
import { PluginRegistry } from '../src/plugins/registry.js';
import { HookBus } from '../src/plugins/hooks.js';
import { PluginHost } from '../src/plugins/host.js';
import { PluginAssetStore } from '../src/plugins/assets.js';
import { missingCapabilities, assertCapabilities, Sandbox, capabilityReport } from '../src/plugins/sandbox.js';
import { runTool, assertArg, assertArgs, toolCommand, listTools } from '../src/pipeline/tools.js';
import { tempRoots, cleanup } from './helpers.js';

const MANIFEST = {
  name: 'seo',
  version: '1.2.0',
  title: 'SEO',
  hooks: ['entry.afterUpdate'],
  permissions: ['content:read'],
  format: 'fragment',
};

async function pluginRoot(roots, name, manifest) {
  const dir = join(roots.plugins, name);
  await mkdir(dir, { recursive: true });
  await writeFile(join(dir, 'plugin.json'), JSON.stringify(manifest));
  return dir;
}

test('manifests are validated', () => {
  const manifest = parseManifest(MANIFEST);
  assert.equal(manifest.name, 'seo');
  assert.equal(manifest.enabled, true);
  assert.equal(manifestSummary(manifest).hooks, 1);
  assert.throws(() => parseManifest({ ...MANIFEST, name: 'Bad Name' }), /lowercase slug/);
  assert.throws(() => parseManifest({ ...MANIFEST, version: '1.2' }), /semver/);
  assert.throws(() => parseManifest({ ...MANIFEST, hooks: ['nope'] }), /unknown hook/);
  assert.throws(() => parseManifest('{'), /not valid JSON/);
  assert.equal(compareVersions('1.2.0', '1.1.9'), 1);
  assert.equal(requiresPermission(manifest, 'content:read'), true);
});

test('the registry keeps plugins in order', () => {
  const registry = new PluginRegistry();
  registry.register(parseManifest(MANIFEST));
  registry.register(parseManifest({ ...MANIFEST, name: 'media-kit', hooks: ['media.afterUpload'] }));
  assert.equal(registry.list().length, 2);
  assert.equal(registry.forHook('entry.afterUpdate').length, 1);
  assert.throws(() => registry.register(parseManifest(MANIFEST)), /already registered/);
  assert.equal(registry.bump('seo', '1.3.0').version, '1.3.0');
  assert.throws(() => registry.bump('seo', '1.0.0'), /does not advance/);
  assert.equal(registry.remove('media-kit'), true);
});

test('hooks run in registration order and pass the payload', async () => {
  const registry = new PluginRegistry();
  registry.register(parseManifest(MANIFEST));
  const bus = new HookBus({ registry });
  const seen = [];
  bus.on('entry.afterUpdate', async (payload) => {
    seen.push(payload.entry.id);
    return payload;
  });
  const out = await bus.emit('entry.afterUpdate', { entry: { id: 'e1' } });
  assert.deepEqual(seen, ['e1']);
  assert.equal(out.plugin_seo, true);
  assert.equal(bus.stats()['entry.afterUpdate'], 1);
  await bus.emitAll(['entry.afterUpdate'], { entry: { id: 'e2' } });
  assert.equal(bus.history().length, 2);
});

test('the host discovers plugins on disk', async () => {
  const roots = await tempRoots();
  try {
    await pluginRoot(roots, 'seo', MANIFEST);
    const host = new PluginHost({ root: roots.plugins });
    const found = await host.discover();
    assert.equal(found.length, 1);
    assert.equal((await host.loadAll()).length, 1);
    assert.equal(host.stats().total, 1);
    assert.equal(host.disable('seo').enabled, false);
    assert.equal(host.enable('seo').enabled, true);
    assert.equal(host.hookCoverage()['entry.afterUpdate'], 1);
    await assert.rejects(() => host.load('missing'), /not found/);
  } finally {
    await cleanup(roots);
  }
});

test('plugin assets read and write plain names', async () => {
  const roots = await tempRoots();
  try {
    const assets = new PluginAssetStore({ root: join(roots.plugins, 'assets') });
    await mkdir(join(roots.plugins, 'assets'), { recursive: true });
    await assets.write('icon.json', '{"icon":1}');
    const asset = await assets.read('icon.json');
    assert.equal(asset.body.toString('utf8'), '{"icon":1}');
    assert.equal((await assets.list()).length, 1);
    assert.equal((await assets.describe('icon.json')).type, 'application/json; charset=utf-8');
    await assert.rejects(() => assets.read('missing.json'), /not found/);
    await assert.rejects(() => assets.write('../escape.json', 'x'), /unsafe path/);
  } finally {
    await cleanup(roots);
  }
});

test('sandbox checks declared permissions', () => {
  const registry = new PluginRegistry();
  registry.register(parseManifest(MANIFEST));
  const sandbox = new Sandbox({ registry });
  assert.equal(sandbox.check('seo', 'content:read'), true);
  assert.throws(() => sandbox.check('seo', 'content:write'), /may not/);
  assert.equal(sandbox.can('seo', 'content:write'), false);
  assert.equal(sandbox.stats().checks, 3);
  assert.equal(sandbox.stats().denied, 2);
  assert.deepEqual(missingCapabilities(parseManifest(MANIFEST)), []);
  assert.equal(capabilityReport(parseManifest(MANIFEST)).length > 0, true);
});

test('tools validate their arguments and run a script', async () => {
  assert.equal(assertArg('plain'), 'plain');
  assert.throws(() => assertArg('a\nb'), /line break/);
  assert.deepEqual(assertArgs(['a', 'b']), ['a', 'b']);
  assert.equal(listTools().length, 3);
  assert.match(toolCommand('markdown', ['src.md']).file, /render\.mjs$/);
  assert.throws(() => toolCommand('missing', []), /unknown tool/);
  const result = await runTool('markdown', ['hello-world', '--format=fragment']);
  assert.match(result.stdout, /data-entry="hello-world"/);
});

test('a plugin without its permissions is refused', () => {
  const manifest = parseManifest({ ...MANIFEST, hooks: ['entry.afterPublish'] });
  assert.deepEqual(missingCapabilities(manifest), ['render:process']);
  assert.throws(() => assertCapabilities(manifest), /needs render:process/);
});
