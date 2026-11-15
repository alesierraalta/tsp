import { readFile, readdir } from 'node:fs/promises';
import { join } from 'node:path';
import { notFound } from '../util/errors.js';
import { assertPlainName, resolveUnder } from '../media/paths.js';
import { parseManifest } from './manifest.js';
import { PluginRegistry } from './registry.js';
import { HookBus } from './hooks.js';

export class PluginHost {
  constructor(options = {}) {
    this.root = options.root;
    if (!this.root) throw new TypeError('a plugin root is required');
    this.registry = options.registry ?? new PluginRegistry();
    this.bus = options.bus ?? new HookBus({ registry: this.registry, logger: options.logger });
    this.errors = [];
  }

  async discover() {
    let entries;
    try {
      entries = await readdir(this.root, { withFileTypes: true });
    } catch (error) {
      if (error.code === 'ENOENT') return [];
      throw error;
    }
    const found = [];
    for (const entry of entries) {
      if (!entry.isDirectory()) continue;
      const manifest = await this.manifestOf(entry.name).catch((error) => {
        this.errors.push({ plugin: entry.name, message: error.message });
        return undefined;
      });
      if (manifest) found.push(manifest);
    }
    return found;
  }

  async manifestOf(name) {
    const clean = assertPlainName(name);
    const file = resolveUnder(this.root, join(clean, 'plugin.json'));
    const raw = await readFile(file, 'utf8').catch(() => {
      throw notFound(`plugin ${name}`);
    });
    return parseManifest(raw, { root: resolveUnder(this.root, clean) });
  }

  async load(name) {
    const manifest = await this.manifestOf(name);
    if (this.registry.has(manifest.name)) {
      return this.registry.get(manifest.name);
    }
    return this.registry.register(manifest);
  }

  async loadAll() {
    const manifests = await this.discover();
    for (const manifest of manifests) {
      if (!this.registry.has(manifest.name)) this.registry.register(manifest);
    }
    return this.registry.list();
  }

  enable(name) {
    const manifest = this.registry.get(name);
    this.registry.replace({ ...manifest, enabled: true });
    return this.registry.get(name);
  }

  disable(name) {
    const manifest = this.registry.get(name);
    this.registry.replace({ ...manifest, enabled: false });
    return this.registry.get(name);
  }

  async run(hook, payload) {
    return this.bus.emit(hook, payload);
  }

  async transform(pluginName, entry) {
    const manifest = this.registry.get(pluginName);
    if (!manifest.enabled) throw new TypeError(`plugin ${pluginName} is disabled`);
    const { runDeclaredTransform } = await import('./transform.js');
    return runDeclaredTransform(manifest, entry);
  }

  stats() {
    return { ...this.registry.stats(), errors: this.errors.length };
  }

  async reload(name) {
    const manifest = await this.manifestOf(name);
    if (!this.registry.has(manifest.name)) return this.registry.register(manifest);
    return this.registry.replace(manifest);
  }

  async manifests() {
    return this.registry.all();
  }

  capabilities(name) {
    const manifest = this.registry.get(name);
    return { name: manifest.name, permissions: [...(manifest.permissions ?? [])] };
  }

  async assetsOf(name) {
    const manifest = this.manifestOf ? this.registry.get(name) : undefined;
    const root = manifest?.root;
    if (!root) return [];
    return this.assetStores?.list?.() ?? [];
  }

  hookCoverage() {
    const out = {};
    for (const manifest of this.registry.enabled()) {
      for (const hook of manifest.hooks) out[hook] = (out[hook] ?? 0) + 1;
    }
    return out;
  }

  describe() {
    return this.registry.list().map((manifest) => ({ ...manifest, hooks: this.registry.get(manifest.name).hooks }));
  }
}
